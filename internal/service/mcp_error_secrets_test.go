package service_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// TestMCPConnectionTest_ErrorsCarryNoURLSecrets (KI-97 security review): the
// test connects with the stored url, and a Go *url.Error quotes the whole
// url (stripping only a userinfo password), so a token user or a query
// credential reached the JSON response. Error texts carry neither, and
// nothing is logged with them.
func TestMCPConnectionTest_ErrorsCarryNoURLSecrets(t *testing.T) {
	const token, key = "ghp_tokenvalue123", "sk-SECRETVALUE456"
	refusing, _ := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil))) // also receives the log package's output
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := newMCPTestService(t, nil, newRecordingMCPStore())
	svc.SetOutboundPolicy(routedPolicy(t,
		map[string]string{"mcp.example.com": "203.0.113.10", "down.example.com": "203.0.113.99"},
		map[string]*httptest.Server{"203.0.113.10": refusing}))

	for _, tt := range []struct {
		name    string
		url     string
		headers map[string]string
	}{
		{"connect fails", "http://" + token + "@down.example.com/mcp?api_key=" + key + "&x=1", nil},
		{"connect fails, password", "http://svc:" + token + "@down.example.com/mcp?token=" + key, nil},
		{"invalid header name", "http://" + token + "@mcp.example.com/mcp?api_key=" + key, map[string]string{"Bad Header": "x"}},
		{"server refuses", "http://" + token + "@mcp.example.com/mcp?api_key=" + key, nil},
	} {
		for _, transport := range []mcp.TransportType{mcp.TransportSSE, mcp.TransportStreamableHTTP} {
			t.Run(tt.name+"/"+string(transport), func(t *testing.T) {
				result, err := svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: transport, URL: tt.url, Headers: tt.headers})
				if err != nil {
					t.Fatalf("TestConnection: %v", err)
				}
				if result.Success || result.Error == "" {
					t.Fatalf("result %+v, want a failed connection with a reason", result)
				}
				t.Logf("error: %s", result.Error)
				for _, secret := range []string{token, key} {
					if strings.Contains(result.Error, secret) {
						t.Fatalf("the error carries a secret: %s", result.Error)
					}
				}
			})
		}
	}
	for _, secret := range []string{token, key} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("the logs carry a secret:\n%s", logs.String())
		}
	}
}
