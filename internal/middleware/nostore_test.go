package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// S7-G review: GET /me/export (a user's personal data) and the other API
// answers carried no Cache-Control, so a browser or proxy could keep a copy.
func TestNoStore(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name:    "an answer without its own Cache-Control is not stored",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			want:    "no-store",
		},
		{
			name: "an error answer is not stored either",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			want: "no-store",
		},
		{
			name: "a handler's own Cache-Control wins",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "no-cache")
				w.WriteHeader(http.StatusOK)
			},
			want: "no-cache",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			NoStore(tt.handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/me/export", http.NoBody))
			if got := w.Header().Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
