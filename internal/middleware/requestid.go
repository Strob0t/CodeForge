// Package middleware provides HTTP middleware for CodeForge.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/Strob0t/CodeForge/internal/logger"
)

const headerRequestID = "X-Request-ID"

// RequestID is HTTP middleware that extracts X-Request-ID from the request
// header or generates a new one. The ID is stored in the context and set
// on the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			id = generateID()
		}

		ctx := logger.WithRequestID(r.Context(), id)
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateID returns a 16-byte random hex string (32 chars).
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
