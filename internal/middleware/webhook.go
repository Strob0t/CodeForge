package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookHMAC returns middleware that validates HMAC-SHA256 webhook signatures.
// The header parameter specifies which HTTP header contains the signature.
// GitHub uses "X-Hub-Signature-256", GitLab uses "X-Gitlab-Token".
func WebhookHMAC(secret, header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				http.Error(w, `{"error":"webhook secret not configured"}`, http.StatusServiceUnavailable)
				return
			}

			sig := r.Header.Get(header)
			if sig == "" {
				http.Error(w, "missing webhook signature", http.StatusUnauthorized)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			if !verifyHMAC(body, sig, secret) {
				http.Error(w, "invalid webhook signature", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// verifyHMAC checks an HMAC-SHA256 signature. Supports both raw hex and
// "sha256=<hex>" prefix formats (GitHub style).
func verifyHMAC(payload []byte, signature, secret string) bool {
	sig := strings.TrimPrefix(signature, "sha256=")
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sigBytes, expected)
}

// WebhookToken returns middleware that validates a static token header (GitLab style).
func WebhookToken(token, header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				http.Error(w, `{"error":"webhook token not configured"}`, http.StatusServiceUnavailable)
				return
			}

			got := r.Header.Get(header)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, fmt.Sprintf("invalid %s token", header), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
