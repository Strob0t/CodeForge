package user

import "testing"

// A key without scopes, nil or empty (an empty list is what a key created
// with "scopes": [] holds once stored and read back), keeps its user's full
// rights; a scoped key has its scopes and admin:all (S10-A review).
func TestAPIKey_HasScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scopes   []string
		required string
		want     bool
	}{
		{"nil scopes, read", nil, ScopeProjectsRead, true},
		{"nil scopes, admin", nil, ScopeAdminAll, true},
		{"empty scopes, read", []string{}, ScopeProjectsRead, true},
		{"empty scopes, admin", []string{}, ScopeAdminAll, true},
		{"own scope", []string{ScopeProjectsRead}, ScopeProjectsRead, true},
		{"other scope of the group", []string{ScopeProjectsRead}, ScopeProjectsWrite, false},
		{"other group", []string{ScopeProjectsRead}, ScopeRunsRead, false},
		{"admin:all satisfies every scope", []string{ScopeAdminAll}, ScopeRunsWrite, true},
		{"second of two scopes", []string{ScopeProjectsRead, ScopeRunsWrite}, ScopeRunsWrite, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := &APIKey{Scopes: tc.scopes}
			if got := k.HasScope(tc.required); got != tc.want {
				t.Fatalf("HasScope(%q) with %v = %v, want %v", tc.required, tc.scopes, got, tc.want)
			}
		})
	}
}
