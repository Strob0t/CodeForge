// Package nats implements the message queue port using NATS JetStream.
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/resilience"
)

const (
	streamName       = "CODEFORGE"
	headerRequestID  = "X-Request-ID"
	headerRetryCount = "Retry-Count"
	maxRetries       = 3
	nakDelay         = 2 * time.Second
)

// Queue implements messagequeue.Queue using NATS JetStream.
type Queue struct {
	nc      *nats.Conn
	js      jetstream.JetStream
	breaker *resilience.Breaker
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
			slog.Info("nats reconnected", "url", nc.ConnectedUrl())
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
		Name:       streamName,
		Subjects:   []string{"tasks.>", "agents.>", "runs.>", "context.>", "repomap.>", "retrieval.>", "graph.>", "conversation.>", "evaluation.>", "benchmark.>", "mcp.>", "a2a.>", "memory.>", "handoff.>", "backends.>", "review.>", "prompt.>"},
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream stream create: %w", err)
	}

	slog.Info("nats connected", "url", url, "stream", streamName)
	return &Queue{nc: nc, js: js}, nil
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
	msg.Header.Set("Nats-Msg-Id", msgID)

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

// Subscribe registers a handler for messages on the given subject.
// Messages are validated against known schemas before processing.
// Failed messages are retried up to maxRetries times, then moved to a DLQ.
// A background goroutine periodically checks consumer health and logs a warning
// if the consumer has been deleted externally (e.g. NATS purge).
func (q *Queue) Subscribe(ctx context.Context, subject string, handler messagequeue.Handler) (func(), error) {
	name := sanitizeConsumerName("codeforge-go-", subject)
	consumerCfg := jetstream.ConsumerConfig{
		Name:              name,
		Durable:           name,
		FilterSubject:     subject,
		AckPolicy:         jetstream.AckExplicitPolicy,
		DeliverGroup:      "codeforge-go",
		AckWait:           90 * time.Second,
		MaxDeliver:        maxRetries + 1,
		MaxAckPending:     100,
		InactiveThreshold: 5 * time.Minute,
	}

	consumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, consumerCfg)
	if err != nil {
		return nil, fmt.Errorf("nats consumer create: %w", err)
	}

	cons, err := consumer.Consume(func(msg jetstream.Msg) {
		// Dispatch to goroutine so slow handlers (HITL approval waits)
		// do not block other messages on the same consumer.
		go q.handleMessage(ctx, msg, handler)
	})
	if err != nil {
		return nil, fmt.Errorf("nats consume: %w", err)
	}

	// Background health check: detect external consumer deletion and recreate.
	healthCtx, healthCancel := context.WithCancel(ctx)
	go q.monitorConsumer(healthCtx, name, &consumerCfg, handler)

	stop := func() {
		healthCancel()
		cons.Stop()
	}

	return stop, nil
}

// monitorConsumer periodically checks that the named consumer still exists.
// If the consumer has been deleted externally (e.g. NATS purge), it logs a
// warning and attempts to recreate the consumer and restart consumption.
// When the consumer is healthy, the pending message count is recorded as
// an OTEL gauge metric for alerting on consumer lag.
func (q *Queue) monitorConsumer(ctx context.Context, name string, cfg *jetstream.ConsumerConfig, handler messagequeue.Handler) {
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

			newConsumer, createErr := q.js.CreateOrUpdateConsumer(ctx, streamName, *cfg)
			if createErr != nil {
				slog.Error("nats consumer recreation failed",
					"consumer", name,
					"error", createErr,
				)
				continue
			}

			_, consumeErr := newConsumer.Consume(func(msg jetstream.Msg) {
				go q.handleMessage(ctx, msg, handler)
			})
			if consumeErr != nil {
				slog.Error("nats consumer re-subscribe failed",
					"consumer", name,
					"error", consumeErr,
				)
				continue
			}

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
		q.moveToDLQ(ctx, msg)
		return
	}

	if err := handler(msgCtx, msg.Subject(), msg.Data()); err != nil {
		// FIX-050: Use JetStream delivery metadata instead of custom Retry-Count
		// header (which was never incremented on NAK redelivery).
		retries := retryCount(hdrs) // fallback for manually-published messages
		if md, mdErr := msg.Metadata(); mdErr == nil && md.NumDelivered > 0 {
			retries = int(md.NumDelivered) - 1 // NumDelivered counts from 1
		}
		slog.Error("message handler failed",
			"subject", msg.Subject(),
			"request_id", logger.RequestID(msgCtx),
			"retry", retries,
			"error", err,
		)

		if retries >= maxRetries {
			q.moveToDLQ(ctx, msg)
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

// moveToDLQ acks the original message and publishes a copy to {subject}.dlq.
func (q *Queue) moveToDLQ(ctx context.Context, msg jetstream.Msg) {
	dlqSubject := msg.Subject() + ".dlq"
	dlqMsg := &nats.Msg{
		Subject: dlqSubject,
		Data:    msg.Data(),
	}
	if hdrs := msg.Headers(); hdrs != nil {
		dlqMsg.Header = hdrs
	}

	if _, err := q.js.PublishMsg(ctx, dlqMsg); err != nil {
		slog.Error("failed to publish to DLQ",
			"dlq_subject", dlqSubject,
			"error", err,
		)
	} else {
		// FIX-049: Include message ID so operators can monitor DLQ accumulation.
		msgID := ""
		if hdrs := msg.Headers(); hdrs != nil {
			msgID = hdrs.Get("Nats-Msg-Id")
		}
		slog.Warn("message moved to DLQ",
			"subject", msg.Subject(),
			"dlq_subject", dlqSubject,
			"msg_id", msgID,
		)
	}

	// Ack the original to remove it from the main stream
	if ackErr := msg.Ack(); ackErr != nil {
		slog.Error("nats ack (dlq) failed", "error", ackErr)
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

func retryCount(hdrs nats.Header) int {
	if hdrs == nil {
		return 0
	}
	val := hdrs.Get(headerRetryCount)
	if val == "" {
		return 0
	}
	n, _ := strconv.Atoi(val)
	return n
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
