// Package nats implements the message queue port using NATS JetStream.
package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/port/notifier"
	"github.com/Strob0t/CodeForge/internal/resilience"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// Delivery semantics (ADR-016): every subscription is a shared durable pull
// consumer; a failed message is retried until JetStream has delivered it
// maxDeliver times and then moved to "{subject}.dlq". The Python worker uses
// the same retry limit (workers/codeforge/nats_subjects.py).
const (
	streamName      = "CODEFORGE"
	headerRequestID = "X-Request-ID"
	maxRetries      = 3
	maxDeliver      = maxRetries + 1
	nakDelay        = 2 * time.Second
	defaultAckWait  = 90 * time.Second
	dlqMonitorName  = "codeforge-go-dlq-monitor"
	// headerOriginalMsgID carries the original Nats-Msg-Id on a DLQ copy
	// (same name in workers/codeforge/nats_subjects.py).
	headerOriginalMsgID = "X-Original-Msg-Id"
	// maxInProgress bounds how long a handler is reported in progress. The
	// slowest Go handler is the HITL approval wait (runtime.approval_timeout_seconds,
	// default 60 s); a handler running ten times longer is treated as hung: its
	// heartbeat stops and JetStream redelivers the message after AckWait instead
	// of the handler holding a MaxAckPending slot forever.
	maxInProgress = 10 * time.Minute
)

// Queue implements messagequeue.Queue using NATS JetStream.
type Queue struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	breaker  *resilience.Breaker
	notifier notifier.Notifier
	// ackWait is how long JetStream waits for an ack before it redelivers a
	// message; handlers report progress three times per ackWait while they run.
	ackWait time.Duration
}

// reconnectOpts returns NATS connection options for automatic reconnection
// and error reporting. Extracted for testability.
func reconnectOpts() []nats.Option {
	return []nats.Option{
		nats.MaxReconnects(60),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				slog.Warn("nats disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("nats reconnected", "url", secrets.RedactURL(nc.ConnectedUrl()))
		}),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			slog.Error("nats async error", "error", err)
		}),
	}
}

// Connect establishes a connection to NATS and ensures the JetStream stream exists.
func Connect(ctx context.Context, url string) (*Queue, error) {
	nc, err := nats.Connect(url, reconnectOpts()...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	// Ensure the stream exists with subjects matching our topic patterns.
	// Duplicates enables JetStream message deduplication via Nats-Msg-Id header.
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:         streamName,
		Subjects:     []string{"tasks.>", "agents.>", "runs.>", "context.>", "repomap.>", "retrieval.>", "graph.>", "conversation.>", "evaluation.>", "benchmark.>", "mcp.>", "a2a.>", "memory.>", "handoff.>", "backends.>", "review.>", "prompt.>"},
		Duplicates:   2 * time.Minute,
		Retention:    jetstream.LimitsPolicy,
		Storage:      jetstream.FileStorage,
		MaxAge:       30 * 24 * time.Hour,     // 30 days
		MaxBytes:     10 * 1024 * 1024 * 1024, // 10 GB safety cap
		MaxMsgs:      5_000_000,
		MaxMsgSize:   4 * 1024 * 1024, // 4 MB per message
		MaxConsumers: 200,
		Discard:      jetstream.DiscardOld,
		Compression:  jetstream.S2Compression,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream stream create: %w", err)
	}

	q := &Queue{nc: nc, js: js, ackWait: defaultAckWait}
	q.startDLQMonitor(ctx)

	slog.Info("nats connected", "url", secrets.RedactURL(url), "stream", streamName)
	return q, nil
}

// startDLQMonitor creates a consumer that subscribes to all dead-letter subjects
// and logs warnings so operators can detect and investigate poisoned messages.
// DLQ subjects are formed by appending ".dlq" to the original subject, so they
// can be 4-6 tokens deep depending on the original subject depth.
func (q *Queue) startDLQMonitor(ctx context.Context) {
	consumer, err := q.ensureDurable(ctx, &jetstream.ConsumerConfig{
		Name:          dlqMonitorName,
		Durable:       dlqMonitorName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxAckPending: 50,
		FilterSubjects: []string{
			"*.*.dlq",       // 2-token originals (e.g. tasks.agent.dlq)
			"*.*.*.dlq",     // 3-token originals (e.g. runs.run.start.dlq)
			"*.*.*.*.dlq",   // 4-token originals (e.g. conversation.run.start.dlq)
			"*.*.*.*.*.dlq", // 5-token originals (e.g. prompt.evolution.reflect.complete.dlq)
		},
	})
	if err != nil {
		slog.Error("failed to create DLQ monitor consumer", "error", err)
		return
	}

	_, consumeErr := consumer.Consume(func(msg jetstream.Msg) {
		var reqID string
		if hdrs := msg.Headers(); hdrs != nil {
			reqID = hdrs.Get(headerRequestID)
		}
		slog.Warn("message dead-lettered",
			"subject", msg.Subject(),
			"request_id", reqID,
			"data_bytes", len(msg.Data()),
		)
		if q.notifier != nil {
			_ = q.notifier.Send(ctx, notifier.Notification{
				Title:   "Dead-lettered message",
				Message: fmt.Sprintf("subject=%s request_id=%s bytes=%d", msg.Subject(), reqID, len(msg.Data())),
				Level:   "warning",
				Source:  "nats.dlq",
			})
		}
		if ackErr := msg.Ack(); ackErr != nil {
			slog.Error("dlq monitor ack failed", "error", ackErr)
		}
	})
	if consumeErr != nil {
		slog.Error("failed to start DLQ monitor consumer", "error", consumeErr)
		return
	}
	slog.Info("DLQ monitor started", "consumer", dlqMonitorName)
}

// SetNotifier attaches an optional notifier for DLQ alerting.
func (q *Queue) SetNotifier(n notifier.Notifier) {
	q.notifier = n
}

// SetBreaker attaches a circuit breaker to the publish path.
func (q *Queue) SetBreaker(b *resilience.Breaker) {
	q.breaker = b
}

// Publish sends a message to the given subject.
// If the context carries a request ID, it is injected as a NATS header.
// W3C trace context (traceparent) is always injected for distributed tracing.
// If a circuit breaker is attached, the publish is wrapped in it.
func (q *Queue) Publish(ctx context.Context, subject string, data []byte) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}

	// Propagate request ID via NATS message header
	if reqID := logger.RequestID(ctx); reqID != "" {
		msg.Header.Set(headerRequestID, reqID)
	}

	// Inject W3C trace context for distributed tracing
	injectTraceContext(ctx, msg.Header)

	publish := func() error {
		_, err := q.js.PublishMsg(ctx, msg)
		if err != nil {
			return fmt.Errorf("nats publish %s: %w", subject, err)
		}
		return nil
	}

	if q.breaker != nil {
		return q.breaker.Execute(publish)
	}
	return publish()
}

// PublishWithDedup sends a message with a Nats-Msg-Id header for JetStream deduplication.
func (q *Queue) PublishWithDedup(ctx context.Context, subject string, data []byte, msgID string) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	msg.Header.Set(nats.MsgIdHdr, msgID)

	if reqID := logger.RequestID(ctx); reqID != "" {
		msg.Header.Set(headerRequestID, reqID)
	}

	// Inject W3C trace context for distributed tracing
	injectTraceContext(ctx, msg.Header)

	publish := func() error {
		_, err := q.js.PublishMsg(ctx, msg)
		if err != nil {
			return fmt.Errorf("nats publish %s: %w", subject, err)
		}
		return nil
	}

	if q.breaker != nil {
		return q.breaker.Execute(publish)
	}
	return publish()
}

// consumerHealthInterval is how often we check that a consumer still exists.
const consumerHealthInterval = 30 * time.Second

// ensureDurable creates the durable consumer described by cfg on first use and
// re-attaches to it afterwards, so every instance can call it on every start.
// A new durable starts at the next published message: creating or recreating
// it never replays the stream history. An existing durable keeps its delivery
// position; its start policy cannot be changed in place, so it is carried
// over and only the mutable settings are updated (this also removes the
// inactivity expiry from durables created by earlier releases).
func (q *Queue) ensureDurable(ctx context.Context, cfg *jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	want := *cfg
	existing, err := q.js.Consumer(ctx, streamName, want.Durable)
	switch {
	case errors.Is(err, jetstream.ErrConsumerNotFound):
		want.DeliverPolicy = jetstream.DeliverNewPolicy
	case err != nil:
		return nil, fmt.Errorf("nats consumer lookup %s: %w", want.Durable, err)
	default:
		current := existing.CachedInfo().Config
		want.DeliverPolicy = current.DeliverPolicy
		want.OptStartSeq = current.OptStartSeq
		want.OptStartTime = current.OptStartTime
	}

	consumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, want)
	if err != nil {
		return nil, fmt.Errorf("nats consumer ensure %s: %w", want.Durable, err)
	}
	return consumer, nil
}

// durableConfig is the consumer configuration of a Go subscription: one
// durable pull consumer per subject, shared by every Go Core instance, so each
// message is processed by one of them.
func (q *Queue) durableConfig(subject string) jetstream.ConsumerConfig {
	name := sanitizeConsumerName("codeforge-go-", subject)
	return jetstream.ConsumerConfig{
		Name:          name,
		Durable:       name,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       q.ackWait,
		MaxDeliver:    maxDeliver,
		MaxAckPending: 100,
	}
}

// consumeHandle holds the active ConsumeContext of a subscription. The health
// monitor replaces it when it recreates a deleted consumer; stop ends whichever
// one is current, including one installed after stop was called.
type consumeHandle struct {
	mu      sync.Mutex
	current jetstream.ConsumeContext
	stopped bool
}

func (h *consumeHandle) replace(next jetstream.ConsumeContext) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		next.Stop()
		return
	}
	previous := h.current
	h.current = next
	h.mu.Unlock()
	if previous != nil {
		previous.Stop()
	}
}

func (h *consumeHandle) stop() {
	h.mu.Lock()
	h.stopped = true
	current := h.current
	h.current = nil
	h.mu.Unlock()
	if current != nil {
		current.Stop()
	}
}

// consume starts delivering the messages of consumer to handler.
func (q *Queue) consume(ctx context.Context, consumer jetstream.Consumer, handler messagequeue.Handler) (jetstream.ConsumeContext, error) {
	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		// Dispatch to goroutine so slow handlers (HITL approval waits)
		// do not block other messages on the same consumer.
		go q.handleMessage(ctx, msg, handler)
	})
	if err != nil {
		return nil, fmt.Errorf("nats consume: %w", err)
	}
	return cc, nil
}

// Subscribe registers a handler for messages on the given subject.
// Messages are validated against known schemas before processing.
// Failed messages are retried until JetStream has delivered them maxDeliver
// times, then moved to a DLQ. A background goroutine periodically checks
// consumer health and recreates the durable if it was deleted externally.
func (q *Queue) Subscribe(ctx context.Context, subject string, handler messagequeue.Handler) (func(), error) {
	cfg := q.durableConfig(subject)
	consumer, err := q.ensureDurable(ctx, &cfg)
	if err != nil {
		return nil, err
	}

	cons, err := q.consume(ctx, consumer, handler)
	if err != nil {
		return nil, err
	}
	handle := &consumeHandle{}
	handle.replace(cons)

	// Background health check: detect external consumer deletion and recreate.
	healthCtx, healthCancel := context.WithCancel(ctx)
	go q.monitorConsumer(healthCtx, &cfg, handler, handle)

	stop := func() {
		healthCancel()
		handle.stop()
	}

	return stop, nil
}

// monitorConsumer periodically checks that the durable consumer still exists.
// If it has been deleted externally (e.g. NATS purge), it logs a warning and
// recreates it through ensureDurable, which starts at new messages instead of
// replaying the stream. When the consumer is healthy, the pending message
// count is recorded as an OTEL gauge metric for alerting on consumer lag.
func (q *Queue) monitorConsumer(ctx context.Context, cfg *jetstream.ConsumerConfig, handler messagequeue.Handler, handle *consumeHandle) {
	name := cfg.Durable
	meter := otel.Meter("codeforge.nats")
	pendingGauge, gaugeErr := meter.Int64Gauge("nats.consumer.pending",
		metric.WithDescription("Number of pending messages for NATS consumer"))
	if gaugeErr != nil {
		slog.Warn("failed to create nats.consumer.pending gauge", "error", gaugeErr)
	}

	ticker := time.NewTicker(consumerHealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cons, err := q.js.Consumer(ctx, streamName, name)
			if err == nil {
				// Record consumer lag metric when healthy.
				if pendingGauge != nil {
					if info, infoErr := cons.Info(ctx); infoErr == nil {
						pendingGauge.Record(ctx, int64(info.NumPending),
							metric.WithAttributes(attribute.String("consumer", name)))
					}
				}
				continue
			}

			slog.Warn("nats consumer unavailable, attempting recreation",
				"consumer", name,
				"error", err,
			)

			newConsumer, createErr := q.ensureDurable(ctx, cfg)
			if createErr != nil {
				slog.Error("nats consumer recreation failed",
					"consumer", name,
					"error", createErr,
				)
				continue
			}

			newCons, consumeErr := q.consume(ctx, newConsumer, handler)
			if consumeErr != nil {
				slog.Error("nats consumer re-subscribe failed",
					"consumer", name,
					"error", consumeErr,
				)
				continue
			}
			handle.replace(newCons)

			slog.Info("nats consumer recreated successfully", "consumer", name)
		}
	}
}

// handleMessage processes a single NATS message with validation, error handling, and ack/nak.
func (q *Queue) handleMessage(ctx context.Context, msg jetstream.Msg, handler messagequeue.Handler) {
	msgCtx := ctx
	hdrs := msg.Headers()
	if hdrs != nil {
		if reqID := hdrs.Get(headerRequestID); reqID != "" {
			msgCtx = logger.WithRequestID(msgCtx, reqID)
		}
		msgCtx = extractTraceContext(msgCtx, hdrs)
	}

	if err := messagequeue.Validate(msg.Subject(), msg.Data()); err != nil {
		slog.Error("message validation failed",
			"subject", msg.Subject(),
			"request_id", logger.RequestID(msgCtx),
			"error", err,
		)
		// An invalid payload fails on every attempt: dead-letter it at once
		// and terminate it so JetStream does not redeliver it.
		q.moveToDLQ(ctx, msg, msg.Term)
		return
	}

	stopProgress := keepInProgress(msg, q.ackWait/3, maxInProgress)
	err := handler(msgCtx, msg.Subject(), msg.Data())
	stopProgress()

	if err != nil {
		// Retries are counted by JetStream (NumDelivered starts at 1).
		var attempt uint64
		md, mdErr := msg.Metadata()
		if mdErr == nil {
			attempt = md.NumDelivered
		}
		slog.Error("message handler failed",
			"subject", msg.Subject(),
			"request_id", logger.RequestID(msgCtx),
			"attempt", attempt,
			"max_deliver", maxDeliver,
			"error", err,
		)

		// Without a delivery count the remaining retries are unknown: dead-letter
		// now instead of letting JetStream drop the message after MaxDeliver.
		if mdErr != nil || attempt >= maxDeliver {
			q.moveToDLQ(ctx, msg, msg.Ack)
			return
		}

		if nakErr := msg.NakWithDelay(nakDelay); nakErr != nil {
			slog.Error("nats nak failed", "error", nakErr)
		}
		return
	}
	if ackErr := msg.Ack(); ackErr != nil {
		slog.Error("nats ack failed", "error", ackErr)
	}
}

// keepInProgress reports msg as in progress every interval while its handler
// runs, for at most limit: a handler slower than AckWait is not redelivered to
// another instance, while a hung one is redelivered once limit plus AckWait
// have passed. It only arms a timer, so a handler that returns within interval
// costs no goroutine and sends nothing. stop waits for a running report to
// finish, so no progress ack follows the final ack.
func keepInProgress(msg jetstream.Msg, interval, limit time.Duration) (stop func()) {
	var (
		mu      sync.Mutex
		stopped bool
		timer   *time.Timer
	)
	deadline := time.Now().Add(limit)
	report := func() {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		if time.Now().After(deadline) {
			slog.Warn("nats handler exceeded the in-progress limit, JetStream will redeliver the message",
				"subject", msg.Subject(),
				"limit", limit,
			)
			return
		}
		if err := msg.InProgress(); err != nil {
			slog.Warn("nats in-progress ack failed", "subject", msg.Subject(), "error", err)
		}
		timer.Reset(interval)
	}

	mu.Lock()
	timer = time.AfterFunc(interval, report)
	mu.Unlock()

	return func() {
		mu.Lock()
		defer mu.Unlock()
		stopped = true
		timer.Stop()
	}
}

// errDuplicateDLQCopy reports a DLQ publish that JetStream discarded as a duplicate.
var errDuplicateDLQCopy = errors.New("dead-letter copy discarded as a duplicate")

// dlqHeaders returns the headers for the dead-letter copy of a message. The
// JetStream publish-control headers (Nats-*) are dropped: with the original
// Nats-Msg-Id the stream would discard the copy as a duplicate of the
// original. The original ID is kept in X-Original-Msg-Id for tracing.
func dlqHeaders(original nats.Header) nats.Header {
	headers := nats.Header{}
	for key, values := range original {
		if strings.HasPrefix(strings.ToLower(key), "nats-") {
			continue
		}
		headers[key] = append([]string(nil), values...)
	}
	if id := original.Get(nats.MsgIdHdr); id != "" {
		headers.Set(headerOriginalMsgID, id)
	}
	return headers
}

// moveToDLQ publishes a copy of msg to {subject}.dlq and then settles the
// original with settle (Ack after the last failed attempt, Term for an invalid
// payload). If no copy was stored (publish error, or a duplicate ack) the
// original is NAK'd instead, so JetStream keeps it (and redelivers it while
// attempts remain) rather than a message being acknowledged without a
// dead-letter copy.
func (q *Queue) moveToDLQ(ctx context.Context, msg jetstream.Msg, settle func() error) {
	dlqSubject := msg.Subject() + ".dlq"
	dlqMsg := &nats.Msg{
		Subject: dlqSubject,
		Data:    msg.Data(),
		Header:  dlqHeaders(msg.Headers()),
	}

	ack, err := q.js.PublishMsg(ctx, dlqMsg)
	if err == nil && ack.Duplicate {
		err = errDuplicateDLQCopy
	}
	if err != nil {
		slog.Error("failed to publish to DLQ, keeping the original message",
			"dlq_subject", dlqSubject,
			"error", err,
		)
		if nakErr := msg.NakWithDelay(nakDelay); nakErr != nil {
			slog.Error("nats nak (dlq) failed", "error", nakErr)
		}
		return
	}

	// FIX-049: Include message ID so operators can monitor DLQ accumulation.
	slog.Warn("message moved to DLQ",
		"subject", msg.Subject(),
		"dlq_subject", dlqSubject,
		"msg_id", dlqMsg.Header.Get(headerOriginalMsgID),
	)

	if settleErr := settle(); settleErr != nil {
		slog.Error("nats settle (dlq) failed", "error", settleErr)
	}
}

// sanitizeConsumerName builds a deterministic durable consumer name from a subject.
//
// FIX-087: Consumer naming convention:
//   - Go consumers: prefix "codeforge-go-" + sanitized subject (dots→dashes, wildcards→"all")
//   - Python consumers: prefix "codeforge-py-" + sanitized subject (see workers/codeforge/consumer/__init__.py)
//   - This ensures unique, deterministic names per language per subject.
//   - Examples: "codeforge-go-conversation-run-start", "codeforge-py-benchmark-run-request"
func sanitizeConsumerName(prefix, subject string) string {
	r := strings.NewReplacer(".", "-", "*", "all", ">", "all")
	return prefix + r.Replace(subject)
}

// natsHeaderCarrier adapts nats.Header to propagation.TextMapCarrier.
type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string { return nats.Header(c).Get(key) }
func (c natsHeaderCarrier) Set(key, value string) { nats.Header(c).Set(key, value) }
func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// injectTraceContext propagates W3C traceparent into NATS message headers.
func injectTraceContext(ctx context.Context, hdrs nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(hdrs))
}

// extractTraceContext reads W3C traceparent from NATS headers into a new context.
func extractTraceContext(ctx context.Context, hdrs nats.Header) context.Context {
	if hdrs == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, natsHeaderCarrier(hdrs))
}

// Drain gracefully drains all subscriptions, waits for pending messages,
// then closes the connection.
func (q *Queue) Drain() error {
	if err := q.nc.Drain(); err != nil {
		return fmt.Errorf("nats drain: %w", err)
	}
	// nc.Drain() is async — wait for the connection to actually close.
	for q.nc.IsConnected() {
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// KeyValue returns a JetStream KeyValue store, creating the bucket if needed.
func (q *Queue) KeyValue(ctx context.Context, bucket string, ttl time.Duration) (jetstream.KeyValue, error) {
	kv, err := q.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucket,
		TTL:    ttl,
	})
	if err != nil {
		return nil, fmt.Errorf("nats kv %s: %w", bucket, err)
	}
	slog.Info("nats kv bucket ready", "bucket", bucket, "ttl", ttl)
	return kv, nil
}

// Close shuts down the NATS connection immediately.
func (q *Queue) Close() error {
	q.nc.Close()
	return nil
}

// IsConnected reports whether the NATS connection is active.
func (q *Queue) IsConnected() bool {
	return q.nc.IsConnected()
}
