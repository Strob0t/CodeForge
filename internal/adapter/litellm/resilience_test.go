package litellm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/resilience"
)

// Chat completions take minutes on real (and local) models: they have their
// own timeout (litellm.completion_timeout), the admin calls keep 10 s
// (KI-213).
func TestClient_CompletionsHaveTheirOwnTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m"}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	if c.httpClient.Timeout != 10*time.Second {
		t.Fatalf("admin timeout = %s, want 10s", c.httpClient.Timeout)
	}
	c.httpClient.Timeout = 50 * time.Millisecond // stands in for the admin 10 s
	c.SetCompletionTimeout(2 * time.Second)

	resp, err := c.ChatCompletion(context.Background(), ChatCompletionRequest{Model: "m"})
	if err != nil {
		t.Fatalf("ChatCompletion within the completion timeout: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content %q", resp.Content)
	}
	if _, err := c.ListModels(context.Background()); err == nil {
		t.Error("an admin call outlived the admin timeout")
	}

	c.SetCompletionTimeout(50 * time.Millisecond)
	if _, err := c.ChatCompletion(context.Background(), ChatCompletionRequest{Model: "m"}); err == nil {
		t.Error("ChatCompletion outlived the completion timeout")
	}
}

// A 4xx answer is the request's fault and never opens the shared LLM breaker;
// 5xx answers do (KI-213).
func TestClient_BreakerCountsOnlyServiceFailures(t *testing.T) {
	tests := []struct {
		status   int
		wantOpen bool
	}{
		{http.StatusBadRequest, false},
		{http.StatusNotFound, false},
		{http.StatusUnprocessableEntity, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/model/new" {
					http.Error(w, "rejected", tt.status)
					return
				}
				_, _ = w.Write([]byte(`{"data":[]}`))
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "")
			c.SetBreaker(resilience.NewBreaker(3, time.Minute))
			for range 5 {
				err := c.AddModel(context.Background(), AddModelRequest{ModelName: "bad"})
				if err == nil {
					t.Fatal("AddModel succeeded")
				}
				if errors.Is(err, resilience.ErrCircuitOpen) {
					break
				}
			}
			_, err := c.ListModels(context.Background())
			if gotOpen := errors.Is(err, resilience.ErrCircuitOpen); gotOpen != tt.wantOpen {
				t.Fatalf("breaker open = %v (ListModels: %v), want %v", gotOpen, err, tt.wantOpen)
			}
		})
	}
}

// The health check asks LiteLLM whether it is ready; /health makes a live
// model call per checked deployment (KI-213).
func TestClient_HealthUsesReadiness(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"healthy","db":"connected"}`))
	}))
	defer srv.Close()

	healthy, err := NewClient(srv.URL, "").Health(context.Background())
	if err != nil || !healthy {
		t.Fatalf("Health = %v, %v", healthy, err)
	}
	if len(paths) != 1 || paths[0] != "/health/readiness" {
		t.Fatalf("paths %v, want [/health/readiness]", paths)
	}
}

// A caller whose own deadline passes (the API route timeout) does not count
// against LiteLLM; the client's own completion timeout does (KI-213).
func TestClient_BreakerIgnoresTheCallersDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"model":"m"}`))
	}))
	defer srv.Close()

	tests := []struct {
		name           string
		callerTimeout  time.Duration
		requestTimeout time.Duration
		wantOpen       bool
	}{
		{"caller deadline", 30 * time.Millisecond, 5 * time.Second, false},
		{"completion timeout", 5 * time.Second, 30 * time.Millisecond, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(srv.URL, "")
			c.SetCompletionTimeout(tt.requestTimeout)
			c.SetBreaker(resilience.NewBreaker(2, time.Minute))
			for range 3 {
				ctx, cancel := context.WithTimeout(context.Background(), tt.callerTimeout)
				_, err := c.ChatCompletion(ctx, ChatCompletionRequest{Model: "m"})
				cancel()
				if err == nil {
					t.Fatal("ChatCompletion outlived its timeout")
				}
			}
			_, err := c.ChatCompletion(context.Background(), ChatCompletionRequest{Model: "m"})
			if gotOpen := errors.Is(err, resilience.ErrCircuitOpen); gotOpen != tt.wantOpen {
				t.Fatalf("breaker open = %v (%v), want %v", gotOpen, err, tt.wantOpen)
			}
		})
	}
}
