package nats

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Backlog tells the quality gate watchdog whether gate requests or results
// are still queued (S3 review finding 4).
func TestBacklog_CountsUnsettledMessagesOfTheSubject(t *testing.T) {
	q := testConnect(t)
	ctx := context.Background()
	subject := uniqueSubject(t)

	if n, err := q.Backlog(ctx, subject); err != nil || n != 0 {
		t.Fatalf("backlog without durables = %d, %v; want 0", n, err)
	}

	cfg := q.durableConfig(subject)
	cons, err := q.ensureDurable(ctx, &cfg)
	if err != nil {
		t.Fatalf("ensureDurable: %v", err)
	}
	for i := range 3 {
		if err := q.Publish(ctx, subject, []byte(fmt.Sprintf(`{"n":%d}`, i))); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if n, err := q.Backlog(ctx, subject); err != nil || n != 3 {
		t.Fatalf("backlog with 3 queued = %d, %v; want 3", n, err)
	}

	// A delivered message counts until it is acked.
	batch, err := cons.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var fetched int
	for msg := range batch.Messages() {
		fetched++
		if n, err := q.Backlog(ctx, subject); err != nil || n != 3 {
			t.Fatalf("backlog with 1 in progress and 2 queued = %d, %v; want 3", n, err)
		}
		if err := msg.DoubleAck(ctx); err != nil {
			t.Fatalf("ack: %v", err)
		}
	}
	if fetched != 1 {
		t.Fatalf("fetched %d messages, want 1", fetched)
	}
	if n, err := q.Backlog(ctx, subject); err != nil || n != 2 {
		t.Fatalf("backlog after one ack = %d, %v; want 2", n, err)
	}
}
