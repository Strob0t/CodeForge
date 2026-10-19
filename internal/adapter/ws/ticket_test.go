package ws

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTicketStore_RedeemOnce(t *testing.T) {
	s := NewTicketStore(time.Minute)
	ticket := s.Issue("user-1", testTenantA)

	got, ok := s.Redeem(ticket)
	if !ok {
		t.Fatal("first Redeem() failed for a fresh ticket")
	}
	if got.UserID != "user-1" || got.TenantID != testTenantA {
		t.Fatalf("claims = (%q, %q), want (user-1, %s)", got.UserID, got.TenantID, testTenantA)
	}
	if _, ok := s.Redeem(ticket); ok {
		t.Fatal("second Redeem() succeeded, want single use")
	}
}

func TestTicketStore_RejectsInvalidTickets(t *testing.T) {
	tests := []struct {
		name   string
		ttl    time.Duration
		tenant string
		redeem func(issued string) string
	}{
		{"empty", time.Minute, testTenantA, func(string) string { return "" }},
		{"unknown", time.Minute, testTenantA, func(string) string { return "not-a-ticket" }},
		{"expired", time.Millisecond, testTenantA, func(tk string) string { return tk }},
		{"without tenant", time.Minute, "", func(tk string) string { return tk }},
		{"padded", time.Minute, testTenantA, func(tk string) string { return " " + tk }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTicketStore(tt.ttl)
			issued := s.Issue("user-1", tt.tenant)
			time.Sleep(5 * time.Millisecond) // lets the 1 ms ticket expire

			if claims, ok := s.Redeem(tt.redeem(issued)); ok {
				t.Fatalf("Redeem() = %+v, want rejection", claims)
			}
		})
	}
}

func TestTicketStore_ConcurrentRedeemSucceedsOnce(t *testing.T) {
	s := NewTicketStore(time.Minute)
	ticket := s.Issue("user-1", testTenantA)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := s.Redeem(ticket); ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("%d redemptions succeeded, want exactly 1", got)
	}
}

func TestTicketStore_RemoveExpired(t *testing.T) {
	s := NewTicketStore(time.Millisecond)
	s.Issue("user-1", testTenantA)
	time.Sleep(5 * time.Millisecond)

	s.removeExpired()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tickets) != 0 {
		t.Fatalf("%d tickets left after cleanup, want 0", len(s.tickets))
	}
}

func TestTicketStore_TTL(t *testing.T) {
	if got := NewTicketStore(DefaultTicketTTL).TTL(); got != DefaultTicketTTL {
		t.Fatalf("TTL() = %s, want %s", got, DefaultTicketTTL)
	}
}
