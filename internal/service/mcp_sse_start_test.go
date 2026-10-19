package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// TestMCPConnectionTest_SSEOpensTheStream: the connection test of an sse
// server called Initialize without starting the mcp-go transport, so it
// failed with "transport not started yet" without ever connecting; no sse
// server could pass the test. The stream is now opened first.
func TestMCPConnectionTest_SSEOpensTheStream(t *testing.T) {
	var streams atomic.Int32
	server, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			streams.Add(1)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	svc := newMCPTestService(t, nil, newRecordingMCPStore())
	svc.SetOutboundPolicy(routedPolicy(t, map[string]string{"mcp.example.com": "203.0.113.10"}, map[string]*httptest.Server{"203.0.113.10": server}))

	result, err := svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: mcp.TransportSSE, URL: "http://mcp.example.com/sse"})
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if result.Success || streams.Load() != 1 || strings.Contains(result.Error, "not started") {
		t.Fatalf("result %+v after %d stream requests, want the stream opened once and its failure reported", result, streams.Load())
	}
}
