package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const wsTicketTenant = "33333333-3333-3333-3333-333333333333"

// recordingTicketStore records the claims tickets are issued for.
type recordingTicketStore struct {
	userID, tenantID string
}

func (s *recordingTicketStore) Issue(userID, tenantID string) string {
	s.userID, s.tenantID = userID, tenantID
	return "ticket-123"
}

func (s *recordingTicketStore) TTL() time.Duration { return 30 * time.Second }

func wsTicketRequest(u *user.User, tenantID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ws/ticket", http.NoBody)
	if u != nil {
		req = withUserContext(req, u)
	}
	if tenantID != "" {
		req = req.WithContext(tenantctx.WithTenant(req.Context(), tenantID))
	}
	return req
}

func TestIssueWSTicket_BindsUserAndTenant(t *testing.T) {
	store := &recordingTicketStore{}
	h := &cfhttp.Handlers{WSTickets: store}
	rec := httptest.NewRecorder()

	h.IssueWSTicket(rec, wsTicketRequest(&user.User{ID: "user-7", TenantID: wsTicketTenant}, wsTicketTenant))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Ticket != "ticket-123" {
		t.Errorf("ticket = %q, want ticket-123", resp.Ticket)
	}
	if resp.ExpiresIn != 30 {
		t.Errorf("expires_in = %d, want the store TTL of 30", resp.ExpiresIn)
	}
	if store.userID != "user-7" || store.tenantID != wsTicketTenant {
		t.Errorf("ticket issued for (%q, %q), want (user-7, %s)", store.userID, store.tenantID, wsTicketTenant)
	}
}

func TestIssueWSTicket_Unauthenticated(t *testing.T) {
	store := &recordingTicketStore{}
	h := &cfhttp.Handlers{WSTickets: store}
	rec := httptest.NewRecorder()

	h.IssueWSTicket(rec, wsTicketRequest(nil, wsTicketTenant))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if store.userID != "" {
		t.Errorf("ticket issued without an authenticated user")
	}
}

func TestIssueWSTicket_StoreNotConfigured(t *testing.T) {
	h := &cfhttp.Handlers{}
	rec := httptest.NewRecorder()

	h.IssueWSTicket(rec, wsTicketRequest(&user.User{ID: "user-7", TenantID: wsTicketTenant}, wsTicketTenant))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
