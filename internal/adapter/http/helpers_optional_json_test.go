package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type optionalBody struct {
	Branch string `json:"branch"`
}

// readOptionalJSON (S10-A review): an absent body is the zero value and
// the handler goes on; a body that is there but not JSON of the type is
// refused with 400, not silently taken as the zero value; a body over the
// limit is refused with 413.
func TestReadOptionalJSON(t *testing.T) {
	const limit = 64
	for _, tc := range []struct {
		name       string
		body       string
		wantOK     bool
		wantBranch string
		wantStatus int
		wantError  string
	}{
		{name: "no body", body: "", wantOK: true},
		{name: "whitespace only", body: " \n\t", wantOK: true},
		{name: "valid", body: `{"branch":"main"}`, wantOK: true, wantBranch: "main"},
		{name: "empty object", body: `{}`, wantOK: true},
		{name: "truncated", body: `{"branch":`, wantStatus: http.StatusBadRequest, wantError: "invalid request body"},
		{name: "not json", body: `branch=main`, wantStatus: http.StatusBadRequest, wantError: "invalid request body"},
		{name: "wrong type", body: `{"branch":1}`, wantStatus: http.StatusBadRequest, wantError: "invalid request body"},
		{name: "over the limit", body: `{"branch":"` + strings.Repeat("x", limit) + `"}`, wantStatus: http.StatusRequestEntityTooLarge, wantError: "request body too large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/clone", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			body, ok := readOptionalJSON[optionalBody](rec, req, limit, "test")
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (status %d, %s)", ok, tc.wantOK, rec.Code, rec.Body.String())
			}
			if tc.wantOK {
				if body.Branch != tc.wantBranch {
					t.Fatalf("branch = %q, want %q", body.Branch, tc.wantBranch)
				}
				if rec.Body.Len() != 0 {
					t.Fatalf("an accepted body wrote an answer: %d %s", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantError) {
				t.Fatalf("status %d (%s), want %d with %q", rec.Code, rec.Body.String(), tc.wantStatus, tc.wantError)
			}
		})
	}
}
