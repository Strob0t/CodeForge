package nats

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// KI-86: two Go Core replicas (two connections) hand a result or decision to
// the one that waits for it.

func TestRelay_QueueOffersTheRelay(t *testing.T) {
	if messagequeue.RelayOf(&Queue{}) == nil {
		t.Fatal("the NATS queue offers no relay")
	}
	if messagequeue.RelayOf(nil) != nil {
		t.Fatal("a missing queue offers a relay")
	}
}

func TestRelay_RequestReachesTheServingReplica(t *testing.T) {
	waiter, other := testConnect(t), testConnect(t)
	ctx := context.Background()
	key := "test:" + t.Name()

	var got []string
	stop, err := waiter.Serve(key, func(data []byte) []byte {
		got = append(got, string(data))
		return append([]byte("took "), data...)
	})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	answer, err := other.Request(ctx, key, []byte("allow"))
	if err != nil || string(answer) != "took allow" {
		t.Fatalf("Request = %q, %v; want the waiter's answer", answer, err)
	}
	if len(got) != 1 || got[0] != "allow" {
		t.Fatalf("waiter got %q", got)
	}

	stop()
	start := time.Now()
	answer, err = other.Request(ctx, key, []byte("allow"))
	if err != nil || answer != nil {
		t.Fatalf("Request after stop = %q, %v; want no answer", answer, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Request without a waiter took %s, want it answered at once (no responders)", d)
	}
}

func TestRelay_Answers(t *testing.T) {
	waiter, other := testConnect(t), testConnect(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		answer []byte
		want   []byte
	}{
		{"no answer", nil, nil},
		{"empty answer", []byte{}, []byte{}},
		{"answer starting with a NUL", []byte{0, 1}, []byte{0, 1}},
		{"large answer", bytes.Repeat([]byte("x"), 64<<10), bytes.Repeat([]byte("x"), 64<<10)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "test:" + t.Name()
			stop, err := waiter.Serve(key, func([]byte) []byte { return tc.answer })
			if err != nil {
				t.Fatalf("Serve: %v", err)
			}
			defer stop()
			answer, err := other.Request(ctx, key, nil)
			if err != nil {
				t.Fatalf("Request: %v", err)
			}
			if (answer == nil) != (tc.want == nil) || !bytes.Equal(answer, tc.want) {
				t.Fatalf("Request = %v, want %v", answer, tc.want)
			}
		})
	}
}

func TestRelay_KeysDoNotCollide(t *testing.T) {
	waiter, other := testConnect(t), testConnect(t)
	ctx := context.Background()
	stop, err := waiter.Serve("approval:run:call@tenant-a", func([]byte) []byte { return []byte("a") })
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer stop()
	for _, key := range []string{"approval:run:call@tenant-b", "approval:run:call@tenant-a ", "approval:run:*@tenant-a", "approval:run:>"} {
		if answer, err := other.Request(ctx, key, nil); err != nil || answer != nil {
			t.Errorf("Request(%q) = %q, %v; want no answer", key, answer, err)
		}
	}
	if answer, _ := other.Request(ctx, "approval:run:call@tenant-a", nil); string(answer) != "a" {
		t.Fatalf("Request of the served key = %q", answer)
	}
}

func TestRelay_SubjectIsOutsideTheStream(t *testing.T) {
	subject := relaySubject("approval:run:call@tenant")
	if !strings.HasPrefix(subject, "core.relay.") || strings.ContainsAny(strings.TrimPrefix(subject, "core.relay."), ".*> \t") {
		t.Fatalf("relaySubject = %q, want one token below core.relay.", subject)
	}
	for _, s := range streamConfig(1).Subjects {
		if strings.HasPrefix(s, "core.") {
			t.Fatalf("the stream captures %s: a request on it would be answered by JetStream", s)
		}
	}
}

func TestRelay_RequestFailsWhenDisconnected(t *testing.T) {
	q := testConnect(t)
	q.nc.Close()
	if _, err := q.Request(context.Background(), "test:closed", nil); err == nil {
		t.Fatal("Request on a closed connection succeeded")
	}
	if _, err := q.Serve("test:closed", func([]byte) []byte { return nil }); err == nil {
		t.Fatal("Serve on a closed connection succeeded")
	}
}

// TestAuth_CoreRelay: with the production NATS configuration the Go Core's
// replicas relay to each other, and the worker can neither request a relay
// key (forge a decision) nor listen to one.
func TestAuth_CoreRelay(t *testing.T) {
	addr := startAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	coreURL := "nats://core:" + authTestCorePassword + "@" + addr
	waiter, err := Connect(ctx, coreURL, 1<<30)
	if err != nil {
		t.Fatalf("core connect: %v", err)
	}
	t.Cleanup(func() { _ = waiter.Close() })
	other, err := Connect(ctx, coreURL, 1<<30)
	if err != nil {
		t.Fatalf("core connect: %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })

	const key = "approval:run-1:call-1@tenant-1"
	stop, err := waiter.Serve(key, func(data []byte) []byte { return append([]byte("took "), data...) })
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer stop()
	if answer, err := other.Request(ctx, key, []byte("allow")); err != nil || string(answer) != "took allow" {
		t.Fatalf("core relay = %q, %v", answer, err)
	}

	nc, asyncErrs := workerConn(t, addr)
	if _, err := nc.Request(relaySubject(key), []byte("allow"), time.Second); err == nil {
		t.Fatal("the worker relayed a decision to the Go Core")
	}
	if !hasPermissionViolation(asyncErrs(), "core.relay") {
		t.Fatalf("no permissions violation for the worker's relay request: %v", asyncErrs())
	}
	sub, err := nc.SubscribeSync("core.relay.>")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Request(ctx, key, []byte("deny")); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.NextMsg(500 * time.Millisecond); err == nil {
		t.Fatal("the worker received a relayed decision")
	}
	if !hasPermissionViolation(asyncErrs(), "core.relay.>") {
		t.Fatalf("no permissions violation for the worker's relay subscription: %v", asyncErrs())
	}
}
