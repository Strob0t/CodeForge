package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// startAuthedHub serves the hub behind the production middleware chain
// (Auth with auth enabled, then TenantID), mounted at /ws like main.go.
func startAuthedHub(t *testing.T, hub *Hub) *httptest.Server {
	t.Helper()
	authSvc := service.NewAuthService(nil, &config.Auth{
		Enabled:            true,
		JWTSecret:          "ws-handshake-test-secret",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: time.Hour,
		BcryptCost:         4,
	})
	chain := middleware.Auth(authSvc, true)(middleware.TenantID(http.HandlerFunc(hub.HandleWS)))
	mux := http.NewServeMux()
	mux.Handle("/ws", chain)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// dialWS dials the /ws endpoint with the given raw query and header and
// returns the HTTP status of the handshake plus the connection on success.
func dialWS(t *testing.T, srv *httptest.Server, rawQuery string, header http.Header) (int, *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	if rawQuery != "" {
		url += "?" + rawQuery
	}
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if err != nil {
		return status, nil
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return status, c
}

func TestHandleWS_ValidTicketConnectsOnlyOnce(t *testing.T) {
	tickets := NewTicketStore(time.Minute)
	hub := NewHub("", tickets)
	srv := startAuthedHub(t, hub)
	ticket := tickets.Issue("user-1", testTenantA)

	status, conn := dialWS(t, srv, "ticket="+ticket, nil)
	if conn == nil || status != http.StatusSwitchingProtocols {
		t.Fatalf("first use of a valid ticket: status %d, want 101", status)
	}
	waitForConnections(t, hub, 1)

	status, conn = dialWS(t, srv, "ticket="+ticket, nil)
	if conn != nil || status != http.StatusUnauthorized {
		t.Fatalf("reused ticket: status %d, want 401", status)
	}
}

func TestHandleWS_RejectsInvalidTickets(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		rawQuery func(ticket string) string
	}{
		{"missing ticket", time.Minute, func(string) string { return "" }},
		{"empty ticket", time.Minute, func(string) string { return "ticket=" }},
		{"unknown ticket", time.Minute, func(string) string { return "ticket=00000000-0000-0000-0000-00000000dead" }},
		{"ticket with whitespace", time.Minute, func(tk string) string { return "ticket=%20" + tk }},
		{"ticket in upper case", time.Minute, func(tk string) string { return "ticket=" + strings.ToUpper(tk) }},
		{"expired ticket", time.Millisecond, func(tk string) string { return "ticket=" + tk }},
		{"ticket passed as token", time.Minute, func(tk string) string { return "token=" + tk }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tickets := NewTicketStore(tt.ttl)
			hub := NewHub("", tickets)
			srv := startAuthedHub(t, hub)
			ticket := tickets.Issue("user-1", testTenantA)
			time.Sleep(5 * time.Millisecond) // lets the 1 ms ticket expire

			status, conn := dialWS(t, srv, tt.rawQuery(ticket), nil)
			if conn != nil || status != http.StatusUnauthorized {
				t.Fatalf("status %d, want 401", status)
			}
			if hub.ConnectionCount() != 0 {
				t.Fatalf("hub registered %d connections, want 0", hub.ConnectionCount())
			}
		})
	}
}

func TestHandleWS_TicketTenantScopesTheConnection(t *testing.T) {
	tickets := NewTicketStore(time.Minute)
	hub := NewHub("", tickets)
	srv := startAuthedHub(t, hub)
	ticket := tickets.Issue("user-1", testTenantA)

	// A spoofed tenant header must not move the connection to tenant B.
	header := http.Header{"X-Tenant-ID": []string{testTenantB}}
	_, conn := dialWS(t, srv, "ticket="+ticket, header)
	if conn == nil {
		t.Fatal("dial with valid ticket failed")
	}
	conn.SetReadLimit(-1)
	waitForConnections(t, hub, 1)

	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), testTenantB), "run.status", map[string]string{"run_id": "run-b"})
	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), testTenantA), "run.status", map[string]string{"run_id": "run-a"})

	msg := readMessage(t, conn)
	if !strings.Contains(string(msg.Payload), "run-a") {
		t.Fatalf("got %s, want only the ticket tenant's event run-a", msg.Payload)
	}
}
