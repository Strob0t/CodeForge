package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// KI-57: the web UI's approval page (linked from approval emails) reads the
// pending tool call with GET /runs/{id}/approvals/{callId}; a call that is
// not pending (answered, timed out, another tenant's) is 404.
func TestGetPendingApproval_NotPending(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/approvals/call-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "no pending approval") {
		t.Fatalf("status %d body %s, want 404 no pending approval", w.Code, w.Body.String())
	}
}

// The approval page decides with POST /feedback/{run}/{call}?decision=;
// the link in the email is never the decision.
func TestFeedbackCallback_DecidesOnlyAPendingCallByPost(t *testing.T) {
	r := newTestRouter()
	for _, tc := range []struct {
		method, query string
		want          int
	}{
		{http.MethodGet, "?decision=allow", http.StatusMethodNotAllowed},
		{http.MethodPost, "?decision=maybe", http.StatusBadRequest},
		{http.MethodPost, "?decision=allow", http.StatusNotFound},
	} {
		req := httptest.NewRequest(tc.method, "/api/v1/feedback/run-1/call-1"+tc.query, http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s %s: status %d body %s, want %d", tc.method, tc.query, w.Code, w.Body.String(), tc.want)
		}
	}
}
