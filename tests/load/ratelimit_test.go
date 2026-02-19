//go:build load

// Package load contains load tests that are excluded from regular CI runs.
// Run with: go test -tags load -count=1 -timeout 60s ./tests/load/
package load

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/middleware"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestRateLimitSustainedLoad runs 10 goroutines x 100 requests from the same
// IP against a rate=10 burst=10 limiter. With 1000 requests completed
// near-instantly, most should be rate-limited since the bucket only starts
// with 10 tokens and refills at 10/sec.
func TestRateLimitSustainedLoad(t *testing.T) {
	rl := middleware.NewRateLimiter(10, 10)
	handler := rl.Handler(okHandler())

	const goroutines = 10
	const reqsPerGoroutine = 100

	var ok, limited atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range reqsPerGoroutine {
				req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
				req.RemoteAddr = "10.0.0.1"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				switch rec.Code {
				case http.StatusOK:
					ok.Add(1)
				case http.StatusTooManyRequests:
					limited.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	total := ok.Load() + limited.Load()
	limitedPct := float64(limited.Load()) / float64(total) * 100
	t.Logf("total=%d ok=%d limited=%d (%.1f%% rejected)", total, ok.Load(), limited.Load(), limitedPct)

	if limited.Load() == 0 {
		t.Error("expected some requests to be rate-limited")
	}
	// With burst=10, rate=10/s, and 1000 requests fired near-instantly,
	// at least 90% should be rejected.
	if limitedPct < 80 {
		t.Errorf("expected >80%% rate-limited under sustained load, got %.1f%%", limitedPct)
	}
}

// TestRateLimitBurstAbsorption verifies that burst-size concurrent requests
// all succeed, and the next request is rejected.
func TestRateLimitBurstAbsorption(t *testing.T) {
	const burstSize = 50
	rl := middleware.NewRateLimiter(1, burstSize)
	handler := rl.Handler(okHandler())

	var ok, limited atomic.Int64
	var wg sync.WaitGroup
	wg.Add(burstSize)

	// Send burstSize concurrent requests from same IP
	for range burstSize {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = "10.0.0.1"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			switch rec.Code {
			case http.StatusOK:
				ok.Add(1)
			case http.StatusTooManyRequests:
				limited.Add(1)
			}
		}()
	}
	wg.Wait()

	t.Logf("burst phase: ok=%d limited=%d", ok.Load(), limited.Load())

	// All burst requests should have succeeded (token bucket starts full)
	if ok.Load() != burstSize {
		t.Errorf("expected all %d burst requests to succeed, got ok=%d limited=%d",
			burstSize, ok.Load(), limited.Load())
	}

	// Next request (burst+1) should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = "10.0.0.1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("burst+1 request: expected 429, got %d", rec.Code)
	}
}

// TestRateLimitPerIPIsolation verifies that 2 IPs have independent buckets.
func TestRateLimitPerIPIsolation(t *testing.T) {
	const rate = 5
	const burst = 5
	rl := middleware.NewRateLimiter(rate, burst)
	handler := rl.Handler(okHandler())

	doRequests := func(ip string, count int) (ok, limited int) {
		for range count {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = ip
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			switch rec.Code {
			case http.StatusOK:
				ok++
			case http.StatusTooManyRequests:
				limited++
			}
		}
		return
	}

	// Exhaust IP1
	ok1, lim1 := doRequests("10.0.0.1", burst+3)
	t.Logf("IP1: ok=%d limited=%d", ok1, lim1)
	if ok1 != burst {
		t.Errorf("IP1: expected %d OK, got %d", burst, ok1)
	}
	if lim1 != 3 {
		t.Errorf("IP1: expected 3 limited, got %d", lim1)
	}

	// IP2 should be unaffected
	ok2, lim2 := doRequests("10.0.0.2", burst)
	t.Logf("IP2: ok=%d limited=%d", ok2, lim2)
	if ok2 != burst {
		t.Errorf("IP2: expected %d OK (independent bucket), got %d", burst, ok2)
	}
	if lim2 != 0 {
		t.Errorf("IP2: expected 0 limited, got %d", lim2)
	}
}

// TestRateLimitConcurrentBucketCreation sends 1 request each from 100 unique
// IPs concurrently and verifies that all succeed and all buckets are created.
func TestRateLimitConcurrentBucketCreation(t *testing.T) {
	const numIPs = 100
	rl := middleware.NewRateLimiter(1, 1)
	handler := rl.Handler(okHandler())

	var wg sync.WaitGroup
	var ok atomic.Int64
	wg.Add(numIPs)

	for i := range numIPs {
		go func(idx int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.%d.%d.%d", idx/65536, (idx/256)%256, idx%256)
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = ip
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if ok.Load() != numIPs {
		t.Errorf("expected all %d first requests to succeed, got %d", numIPs, ok.Load())
	}
	if rl.Len() != numIPs {
		t.Errorf("expected %d buckets, got %d", numIPs, rl.Len())
	}
}

// TestRateLimitHeadersUnderLoad verifies that Retry-After is set on 429 and
// X-RateLimit-Remaining is set on 200 across concurrent requests.
func TestRateLimitHeadersUnderLoad(t *testing.T) {
	rl := middleware.NewRateLimiter(5, 5)
	handler := rl.Handler(okHandler())

	// First 5 requests succeed with X-RateLimit-Remaining
	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "10.0.0.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
		remaining := rec.Header().Get("X-RateLimit-Remaining")
		if remaining == "" {
			t.Errorf("request %d: missing X-RateLimit-Remaining", i)
		}
	}

	// Next requests should be rate-limited with Retry-After
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "10.0.0.1"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", rec.Code)
		}
		retryAfter := rec.Header().Get("Retry-After")
		if retryAfter == "" {
			t.Error("expected Retry-After header on 429")
		}
	}
}

// TestRateLimitCleanupUnderLoad creates 1000 buckets, then triggers cleanup
// with maxIdle=0 and verifies all buckets are removed.
func TestRateLimitCleanupUnderLoad(t *testing.T) {
	const numBuckets = 1000
	rl := middleware.NewRateLimiter(10, 10)
	handler := rl.Handler(okHandler())

	// Create buckets from 1000 unique IPs
	for i := range numBuckets {
		ip := fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if rl.Len() != numBuckets {
		t.Fatalf("expected %d buckets, got %d", numBuckets, rl.Len())
	}

	// Wait a tiny bit so buckets become stale
	time.Sleep(10 * time.Millisecond)

	// Cleanup with maxIdle=1ms — all buckets older than 1ms get removed
	cancel := rl.StartCleanup(5*time.Millisecond, 1*time.Millisecond)
	defer cancel()

	// Wait for cleanup to run
	time.Sleep(50 * time.Millisecond)

	if rl.Len() != 0 {
		t.Errorf("expected 0 buckets after cleanup, got %d", rl.Len())
	}
}
