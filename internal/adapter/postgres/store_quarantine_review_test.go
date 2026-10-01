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

// accountlessUserIDs are request identities without a row in users: the
// default user while auth is disabled and the internal service key user.
var accountlessUserIDs = []string{
	"00000000-0000-0000-0000-000000000000",
	"00000000-0000-0000-0000-000000000001",
}

// S6-H review 1: reviewed_by_user_id references users, and a reviewer without
// an account (auth disabled, internal service key) has no row. The review is
// recorded with the reviewer's name and no user ID instead of failing on the
// foreign key.
func TestStore_UpdateQuarantineStatus_AccountlessReviewer(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))
	for _, id := range accountlessUserIDs {
		t.Run(id, func(t *testing.T) {
			msgID := reviewQuarantined(ctx, t, store, &user.User{ID: id, Name: "Admin"})
			got, err := store.GetQuarantinedMessage(ctx, msgID)
			if err != nil {
				t.Fatalf("GetQuarantinedMessage: %v", err)
			}
			if got.Status != quarantine.StatusApproved || got.ReviewedByID != "" || got.ReviewedBy != "Admin" || got.ReviewNote != "checked" {
				t.Fatalf("review = %s by %q (%q) %q, want approved by Admin without a user ID",
					got.Status, got.ReviewedByID, got.ReviewedBy, got.ReviewNote)
			}
		})
	}
}
