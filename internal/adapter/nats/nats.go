// Package nats implements the message queue port using NATS JetStream.
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

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
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"tasks.>", "agents.>", "runs.>", "context.>", "repomap.>"},
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
// If a circuit breaker is attached, the publish is wrapped in it.
func (q *Queue) Publish(ctx context.Context, subject string, data []byte) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
	}

	// Propagate request ID via NATS message header
	if reqID := logger.RequestID(ctx); reqID != "" {
		msg.Header = nats.Header{}
		msg.Header.Set(headerRequestID, reqID)
	}

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
	consumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("nats consumer create: %w", err)
	}

	cons, err := consumer.Consume(func(msg jetstream.Msg) {
		// Extract request ID from NATS headers into context
		msgCtx := ctx
		hdrs := msg.Headers()
		if hdrs != nil {
			if reqID := hdrs.Get(headerRequestID); reqID != "" {
				msgCtx = logger.WithRequestID(msgCtx, reqID)
			}
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
		// Spin briefly; Drain closes the connection after flushing.
	}
	return nil
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
