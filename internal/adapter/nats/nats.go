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

// Connect establishes a connection to NATS and ensures the JetStream stream exists.
func Connect(ctx context.Context, url string) (*Queue, error) {
	nc, err := nats.Connect(url)
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

// Subscribe registers a handler for messages on the given subject.
// Messages are validated against known schemas before processing.
// Failed messages are retried up to maxRetries times, then moved to a DLQ.
func (q *Queue) Subscribe(ctx context.Context, subject string, handler messagequeue.Handler) (func(), error) {
	name := sanitizeConsumerName("codeforge-go-", subject)
	consumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Name:              name,
		Durable:           name,
		FilterSubject:     subject,
		AckPolicy:         jetstream.AckExplicitPolicy,
		DeliverGroup:      "codeforge-go",
		AckWait:           90 * time.Second,
		MaxDeliver:        maxRetries + 1,
		MaxAckPending:     100,
		InactiveThreshold: 5 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("nats consumer create: %w", err)
	}

	cons, err := consumer.Consume(func(msg jetstream.Msg) {
		// Extract request ID and trace context from NATS headers
		msgCtx := ctx
		hdrs := msg.Headers()
		if hdrs != nil {
			if reqID := hdrs.Get(headerRequestID); reqID != "" {
				msgCtx = logger.WithRequestID(msgCtx, reqID)
			}
			msgCtx = extractTraceContext(msgCtx, hdrs)
		}

		// Schema validation — reject invalid messages immediately to DLQ
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
			retries := retryCount(hdrs)
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
	})
	if err != nil {
		return nil, fmt.Errorf("nats consume: %w", err)
	}

	return cons.Stop, nil
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
		slog.Warn("message moved to DLQ",
			"subject", msg.Subject(),
			"dlq_subject", dlqSubject,
		)
	}

	// Ack the original to remove it from the main stream
	if ackErr := msg.Ack(); ackErr != nil {
		slog.Error("nats ack (dlq) failed", "error", ackErr)
	}
}

// sanitizeConsumerName builds a deterministic durable consumer name from a subject.
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
