package postgres_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

func reviewQuarantined(ctx context.Context, t *testing.T, store *postgres.Store, reviewer *user.User) string {
	t.Helper()
	now := time.Now().UTC()
	msg := &quarantine.Message{
		ProjectID: "proj-quarantine", Subject: "runs.start", Payload: []byte(`{}`),
		RiskFactors: []string{"path_traversal"}, Status: quarantine.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := store.QuarantineMessage(ctx, msg); err != nil {
		t.Fatalf("QuarantineMessage: %v", err)
	}
	review := &quarantine.Review{ReviewerID: reviewer.ID, ReviewerName: reviewer.Name, Note: "checked"}
	if err := store.UpdateQuarantineStatus(ctx, msg.ID, quarantine.StatusApproved, review); err != nil {
		t.Fatalf("UpdateQuarantineStatus: %v", err)
	}
	return msg.ID
}

// KI-79: a quarantine review records the logged-in reviewer's user ID and
// name; erasing the user replaces the name with a placeholder and unlinks the
// ID, the decision and note stay. Other reviewers' rows are untouched.
func TestGDPRErasure_QuarantineReviews(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	erased := createAuditTestUser(t, store, tenantID)
	erased.Name = "Erased Reviewer"
	kept := createAuditTestUser(t, store, tenantID)
	erasedReview := reviewQuarantined(ctx, t, store, erased)
	keptReview := reviewQuarantined(ctx, t, store, kept)

	got, err := store.GetQuarantinedMessage(ctx, erasedReview)
	if err != nil {
		t.Fatalf("GetQuarantinedMessage: %v", err)
	}
	if got.ReviewedByID != erased.ID || got.ReviewedBy != "Erased Reviewer" {
		t.Fatalf("review = by %q (%q), want %q (%q)", got.ReviewedByID, got.ReviewedBy, erased.ID, "Erased Reviewer")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/data", http.NoBody).
		WithContext(middleware.ContextWithTestUser(ctx, erased))
	rec := httptest.NewRecorder()
	(&cfhttp.Handlers{GDPR: service.NewGDPRService(store)}).DeleteMyData(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /me/data = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	got, err = store.GetQuarantinedMessage(ctx, erasedReview)
	if err != nil {
		t.Fatalf("GetQuarantinedMessage after erasure: %v", err)
	}
	if got.ReviewedByID != "" || got.ReviewedBy != quarantine.ErasedReviewerName {
		t.Errorf("erased reviewer's review = by %q (%q), want no ID and %q", got.ReviewedByID, got.ReviewedBy, quarantine.ErasedReviewerName)
	}
	if got.Status != quarantine.StatusApproved || got.ReviewNote != "checked" {
		t.Errorf("erased reviewer's decision = %s %q, want approved with its note", got.Status, got.ReviewNote)
	}

	listed, err := store.ListQuarantinedMessages(ctx, "proj-quarantine", "", 10, 0)
	if err != nil {
		t.Fatalf("ListQuarantinedMessages: %v", err)
	}
	for _, m := range listed {
		if m.ID == keptReview && (m.ReviewedByID != kept.ID || m.ReviewedBy != kept.Name) {
			t.Errorf("other reviewer's review = by %q (%q), want unchanged", m.ReviewedByID, m.ReviewedBy)
		}
	}
}
