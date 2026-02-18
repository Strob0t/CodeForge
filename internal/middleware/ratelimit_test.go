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
