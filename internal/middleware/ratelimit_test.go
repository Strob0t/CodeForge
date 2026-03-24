package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(10, 10)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 10 requests should succeed (burst = 10)
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "192.168.1.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
}

func TestRateLimiterRejectsOverLimit(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the burst (5 tokens)
	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "192.168.1.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = "192.168.1.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimiterSetsHeaders(t *testing.T) {
	rl := NewRateLimiter(10, 10)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = "192.168.1.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
	if rec.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset header")
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	rl := NewRateLimiter(10, 2)
	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust tokens for IP 1
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "10.0.0.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP 1 should be rate limited
	req1 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req1.RemoteAddr = "10.0.0.1"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusTooManyRequests {
		t.Errorf("IP 10.0.0.1: expected 429, got %d", rec1.Code)
	}

	// IP 2 should still be allowed
	req2 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req2.RemoteAddr = "10.0.0.2"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("IP 10.0.0.2: expected 200, got %d", rec2.Code)
	}
}

func TestRateLimiterLen(t *testing.T) {
	rl := NewRateLimiter(10, 10)

	if rl.Len() != 0 {
		t.Fatalf("expected 0, got %d", rl.Len())
	}

	// Generate traffic from 3 IPs
	for i := range 3 {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		rl.allow(ip)
	}

	if rl.Len() != 3 {
		t.Fatalf("expected 3, got %d", rl.Len())
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(10, 10)

	// Populate buckets for 5 IPs
	for i := range 5 {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		rl.allow(ip)
	}
	if rl.Len() != 5 {
		t.Fatalf("expected 5 buckets, got %d", rl.Len())
	}

	// Manually backdate some buckets to simulate staleness
	rl.mu.Lock()
	staleTime := time.Now().Add(-20 * time.Minute)
	for ip, b := range rl.buckets {
		if ip == "10.0.0.1" || ip == "10.0.0.2" {
			b.lastSeen = staleTime
		}
	}
	rl.mu.Unlock()

	// Cleanup with 10m maxIdle — should remove the 2 stale ones
	rl.cleanup(10 * time.Minute)

	if rl.Len() != 3 {
		t.Fatalf("expected 3 buckets after cleanup, got %d", rl.Len())
	}
}

func TestRateLimiterStartCleanupStops(t *testing.T) {
	rl := NewRateLimiter(10, 10)
	cancel := rl.StartCleanup(50*time.Millisecond, 1*time.Millisecond)

	// Add a bucket and let cleanup run
	rl.allow("10.0.0.1")
	time.Sleep(150 * time.Millisecond)

	// Bucket should have been cleaned up (lastSeen > 1ms ago)
	if rl.Len() != 0 {
		t.Fatalf("expected 0 buckets after cleanup, got %d", rl.Len())
	}

	cancel()
}

func TestRateLimiter_PerUserKey(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 req/s, burst 1

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from user-A on IP 1.2.3.4 — should pass
	r1 := httptest.NewRequest("GET", "/", http.NoBody)
	r1.RemoteAddr = "1.2.3.4:1234"
	r1 = r1.WithContext(withUserID(r1.Context(), "user-A"))
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", w1.Code)
	}

	// Second request from user-B on same IP — should pass (different user)
	r2 := httptest.NewRequest("GET", "/", http.NoBody)
	r2.RemoteAddr = "1.2.3.4:1234"
	r2 = r2.WithContext(withUserID(r2.Context(), "user-B"))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("different user same IP: got %d, want 200", w2.Code)
	}

	// Third request from user-A again on same IP — should be rate limited
	r3 := httptest.NewRequest("GET", "/", http.NoBody)
	r3.RemoteAddr = "1.2.3.4:1234"
	r3 = r3.WithContext(withUserID(r3.Context(), "user-A"))
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, r3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("repeat user-A: got %d, want 429", w3.Code)
	}
}

func TestRateLimiter_UnauthenticatedFallsBackToIP(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 req/s, burst 1

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First unauthenticated request from IP — should pass
	r1 := httptest.NewRequest("GET", "/", http.NoBody)
	r1.RemoteAddr = "5.6.7.8:5678"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first unauth request: got %d, want 200", w1.Code)
	}

	// Second unauthenticated request from same IP — should be rate limited
	r2 := httptest.NewRequest("GET", "/", http.NoBody)
	r2.RemoteAddr = "5.6.7.8:5678"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second unauth request: got %d, want 429", w2.Code)
	}
}

func TestRateLimitKey(t *testing.T) {
	tests := []struct {
		name   string
		ip     string
		userID string
		want   string
	}{
		{"unauthenticated", "10.0.0.1:1234", "", "10.0.0.1"},
		{"authenticated", "10.0.0.1:1234", "user-123", "user-123:10.0.0.1"},
		{"empty user ID", "10.0.0.1:1234", "", "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", http.NoBody)
			r.RemoteAddr = tt.ip
			if tt.userID != "" {
				r = r.WithContext(withUserID(r.Context(), tt.userID))
			}
			got := rateLimitKey(r)
			if got != tt.want {
				t.Errorf("rateLimitKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserIDFromContext(t *testing.T) {
	// Empty context returns empty string.
	if got := userIDFromContext(httptest.NewRequest("GET", "/", http.NoBody).Context()); got != "" {
		t.Errorf("empty context: got %q, want empty", got)
	}

	// Context with user ID returns it.
	ctx := withUserID(httptest.NewRequest("GET", "/", http.NoBody).Context(), "test-user")
	if got := userIDFromContext(ctx); got != "test-user" {
		t.Errorf("with user ID: got %q, want %q", got, "test-user")
	}
}

func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewRateLimiter(1000, 1000)
	b.ResetTimer()
	for i := range b.N {
		ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
		rl.allow(ip)
	}
}

func BenchmarkRateLimiterConcurrent(b *testing.B) {
	rl := NewRateLimiter(1000, 1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
			rl.allow(ip)
			i++
		}
	})
}
