package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	headerIdempotencyKey = "Idempotency-Key"
	maxIdempotencyBody   = 1 << 20 // 1 MB
)

// idempotencyEntry stores a cached HTTP response.
type idempotencyEntry struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body"`
}

// Idempotency returns middleware that deduplicates POST/PUT/DELETE requests
// using the Idempotency-Key header and a NATS JetStream KV store.
func Idempotency(kv jetstream.KeyValue) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply to mutating methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(headerIdempotencyKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check KV for existing response
			entry, err := kv.Get(r.Context(), key)
			if err == nil {
				// Cache hit — replay response
				var cached idempotencyEntry
				if err := json.Unmarshal(entry.Value(), &cached); err == nil {
					for k, vals := range cached.Headers {
						for _, v := range vals {
							w.Header().Add(k, v)
						}
					}
					w.WriteHeader(cached.StatusCode)
					_, _ = w.Write(cached.Body)
					return
				}
				slog.Warn("idempotency: corrupt cache entry", "key", key)
			}

			// Cache miss — process request and capture response
			rec := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           &bytes.Buffer{},
			}
			next.ServeHTTP(rec, r)

			// Store response in KV (best-effort, capped at 1MB)
			if rec.body.Len() <= maxIdempotencyBody {
				cached := idempotencyEntry{
					StatusCode: rec.statusCode,
					Headers:    w.Header().Clone(),
					Body:       rec.body.Bytes(),
				}
				data, marshalErr := json.Marshal(cached)
				if marshalErr == nil {
					if _, putErr := kv.Put(r.Context(), key, data); putErr != nil {
						slog.Warn("idempotency: failed to store response", "key", key, "error", putErr)
					}
				}
			}
		})
	}
}

// responseRecorder wraps http.ResponseWriter to capture the response.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
