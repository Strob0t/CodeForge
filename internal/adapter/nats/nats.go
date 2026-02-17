// Package nats implements the message queue port using NATS JetStream.
package nats

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

const (
	streamName      = "CODEFORGE"
	headerRequestID = "X-Request-ID"
)

// Queue implements messagequeue.Queue using NATS JetStream.
type Queue struct {
	nc *nats.Conn
	js jetstream.JetStream
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
		Subjects: []string{"tasks.>", "agents.>"},
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream stream create: %w", err)
	}

	slog.Info("nats connected", "url", url, "stream", streamName)
	return &Queue{nc: nc, js: js}, nil
}

// Publish sends a message to the given subject.
// If the context carries a request ID, it is injected as a NATS header.
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

	_, err := q.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("nats publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe registers a handler for messages on the given subject.
// The request ID from NATS headers is extracted and passed via context.
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
		if hdrs := msg.Headers(); hdrs != nil {
			if reqID := hdrs.Get(headerRequestID); reqID != "" {
				msgCtx = logger.WithRequestID(msgCtx, reqID)
			}
		}

		if err := handler(msgCtx, msg.Subject(), msg.Data()); err != nil {
			slog.Error("message handler failed",
				"subject", msg.Subject(),
				"request_id", logger.RequestID(msgCtx),
				"error", err,
			)
			if nakErr := msg.Nak(); nakErr != nil {
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

// Close shuts down the NATS connection.
func (q *Queue) Close() error {
	q.nc.Close()
	return nil
}
