package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A Core whose HTTP server cannot bind (the port is taken) must fail at
// startup instead of logging and consuming NATS work without an API (KI-213).
func TestServeHTTP_BindFailureIsAnError(t *testing.T) {
	taken, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()

	srv := &http.Server{Addr: taken.Addr().String(), ReadHeaderTimeout: time.Second}
	errs, err := serveHTTP(context.Background(), srv)
	if err == nil {
		_ = srv.Close()
		t.Fatal("serveHTTP bound a port that is in use")
	}
	if errs != nil {
		t.Error("no serve channel on a bind failure")
	}
	if !strings.Contains(err.Error(), taken.Addr().String()) {
		t.Errorf("error %q does not name the address", err)
	}
}

func TestServeHTTP_ServesUntilShutdown(t *testing.T) {
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	}
	errs, err := serveHTTP(context.Background(), srv)
	if err != nil {
		t.Fatalf("serveHTTP: %v", err)
	}
	if srv.Addr == "127.0.0.1:0" {
		t.Fatal("serveHTTP did not record the bound address")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+srv.Addr+"/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status %d, want 204", resp.StatusCode)
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errs:
		t.Fatalf("a shutdown is not a serve failure: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}
