package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// The Go Core's services find the relay on their queue (messagequeue.RelayOf).
var _ messagequeue.Relay = (*Queue)(nil)

// The relay between Go Core replicas (KI-86, messagequeue.Relay) is core
// NATS request-reply, outside the CODEFORGE stream: a waiter subscribes to
// its key's subject while it waits, and a replica holding a result for it
// sends a request. Only the Go Core's NATS user may publish on these
// subjects (configs/nats/nats-server.conf), but the server does not check a
// push consumer's deliver subject or the reply subject of a JetStream API
// request, and the subject follows from the key: the worker can have a
// message it published to the stream delivered here. Such a delivery
// carries a $JS.ACK.* reply or none, while a replica's request carries a
// reply in the Go Core's inboxes (inboxPrefix), so Serve acts only on those.
const (
	relaySubjectPrefix = "core.relay."
	// relayTimeout bounds a relay request and the subscription's flush.
	relayTimeout = 5 * time.Second
)

// The first byte of a relay reply says whether the waiter answered.
const (
	relayNoAnswer byte = 0
	relayAnswered byte = 1
)

// relaySubject is the subject of a relay key: a hash, so a key may contain
// any character (tenant, run and tool call IDs) and is one subject token.
func relaySubject(key string) string {
	sum := sha256.Sum256([]byte(key))
	return relaySubjectPrefix + hex.EncodeToString(sum[:])
}

// Serve answers the relay requests for key until stop is called. The
// subscription reaches the server before Serve returns, so a result relayed
// right after the waiter's request went out is not missed.
func (q *Queue) Serve(key string, handle func(data []byte) []byte) (func(), error) {
	sub, err := q.nc.Subscribe(relaySubject(key), func(m *nats.Msg) {
		if !isCoreRequest(m) {
			// The payload stays out of the log: it is the forger's.
			slog.Warn("relay message without a Go Core reply inbox dropped", "subject", m.Subject, "reply", m.Reply)
			return
		}
		reply := []byte{relayNoAnswer}
		if answer := handle(m.Data); answer != nil {
			reply = append([]byte{relayAnswered}, answer...)
		}
		if err := m.Respond(reply); err != nil {
			slog.Warn("relay reply not sent", "error", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("relay subscribe: %w", err)
	}
	if err := q.nc.FlushTimeout(relayTimeout); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("relay subscribe: %w", err)
	}
	// The unsubscribe reaches the server before stop returns: a request sent
	// afterwards gets no responders at once instead of waiting for a reply
	// that never comes.
	return func() {
		_ = sub.Unsubscribe()
		_ = q.nc.FlushTimeout(relayTimeout)
	}, nil
}

// isCoreRequest reports whether m is a request of a Go Core replica: its
// reply is in the Go Core's inboxes, which only the core user's requests
// name (every replica connects with connectOpts).
func isCoreRequest(m *nats.Msg) bool {
	return strings.HasPrefix(m.Reply, inboxPrefix+".")
}

// Request sends data to the replica serving key and returns its answer; nil
// at once when no replica serves it (no responders).
func (q *Queue) Request(ctx context.Context, key string, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, relayTimeout)
	defer cancel()
	msg, err := q.nc.RequestWithContext(ctx, relaySubject(key), data)
	if errors.Is(err, nats.ErrNoResponders) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("relay request: %w", err)
	}
	if len(msg.Data) == 0 || msg.Data[0] != relayAnswered {
		return nil, nil
	}
	return msg.Data[1:], nil
}
