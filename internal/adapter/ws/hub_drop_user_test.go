package ws

import (
	"context"
	"testing"
	"time"
)

// KI-143 review: when a user's tokens are invalidated, the user's open
// WebSocket connections are closed; other users keep theirs.
func TestHubDropUser_ClosesOnlyThatUsersConnections(t *testing.T) {
	hub := newTestHub()
	first, second, other, anonymous := newStalledSocket(), newStalledSocket(), newStalledSocket(), newStalledSocket()
	hub.register(context.Background(), first, testTenantA, "user-1")
	hub.register(context.Background(), second, testTenantB, "user-1")
	hub.register(context.Background(), other, testTenantA, "user-2")
	hub.register(context.Background(), anonymous, testTenantA, "") // no user (auth disabled)

	if n := hub.DropUser("user-1"); n != 2 {
		t.Fatalf("DropUser closed %d connections, want 2", n)
	}
	waitClosed(t, first, time.Second)
	waitClosed(t, second, time.Second)
	if got := hub.ConnectionCount(); got != 2 {
		t.Fatalf("hub has %d connections, want 2", got)
	}
	select {
	case <-other.closed:
		t.Fatal("another user's connection was closed")
	default:
	}

	if n := hub.DropUser("user-1"); n != 0 {
		t.Fatalf("second DropUser closed %d, want 0", n)
	}
	if n := hub.DropUser(""); n != 0 || hub.ConnectionCount() != 2 {
		t.Fatalf("DropUser(\"\") closed %d, want 0 (connections without a user stay)", n)
	}
}
