package middleware

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter is per-IP token bucket rate limiting middleware.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	rate       float64 // tokens per second
	burst      int     // max tokens
	maxBuckets int     // max tracked IPs (prevents memory exhaustion)
}

type bucket struct {
	tokens    float64
	lastSeen  time.Time
	updatedAt time.Time
}

// NewRateLimiter creates a rate limiter with the given sustained rate
// (requests per second) and burst size.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		buckets:    make(map[string]*bucket),
		rate:       rate,
		burst:      burst,
		maxBuckets: 100000, // 100k IPs max
	}
}

// Handler returns HTTP middleware that enforces per-IP rate limiting.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)

		remaining, retryAfter, allowed := rl.allow(ip)

		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Second).Unix()))

		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", math.Ceil(retryAfter)))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allow checks whether a request from the given IP is allowed.
// Returns remaining tokens, seconds until next token, and whether the request is allowed.
func (rl *RateLimiter) allow(ip string) (remaining int, retryAfter float64, allowed bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[ip]
	if !exists {
		// Prevent memory exhaustion: cap the number of tracked IPs
		if len(rl.buckets) >= rl.maxBuckets {
			return 0, 1.0 / rl.rate, false // reject when at capacity
		}
		b = &bucket{
			tokens:    float64(rl.burst) - 1, // consume one token for this request
			updatedAt: now,
			lastSeen:  now,
		}
		rl.buckets[ip] = b
		return int(b.tokens), 0, true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.updatedAt).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.updatedAt = now
	b.lastSeen = now

	if b.tokens < 1 {
		// Not enough tokens — calculate wait time
		wait := (1 - b.tokens) / rl.rate
		return 0, wait, false
	}

	b.tokens--
	return int(b.tokens), 0, true
}

// StartCleanup spawns a goroutine that removes stale buckets every interval.
// A bucket is stale if it has not been seen for longer than maxIdle.
// Returns a cancel function that stops the cleanup goroutine.
func (rl *RateLimiter) StartCleanup(interval, maxIdle time.Duration) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.cleanup(maxIdle)
			}
		}
	}()
	return cancel
}

// cleanup removes buckets that have been idle longer than maxIdle.
func (rl *RateLimiter) cleanup(maxIdle time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-maxIdle)
	for ip, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
}

// Len returns the number of tracked IP buckets (for metrics and testing).
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

// realIP extracts the client IP from RemoteAddr.
// Proxy headers (X-Forwarded-For, X-Real-Ip) are NOT trusted because
// they can be spoofed by attackers to bypass rate limiting.
func realIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
