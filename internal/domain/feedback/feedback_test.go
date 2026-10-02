package feedback

import "testing"

// KI-84: an operator-configured approval channel receives only the requests
// of its configured tenants; a request without a tenant never.
func TestSendsTo(t *testing.T) {
	const a, b = "aaaaaaaa-0000-4000-8000-000000000001", "bbbbbbbb-0000-4000-8000-000000000002"
	tests := []struct {
		name     string
		tenants  []string
		tenantID string
		want     bool
	}{
		{"configured tenant", []string{a}, a, true},
		{"second configured tenant", []string{a, b}, b, true},
		{"other tenant", []string{a}, b, false},
		{"request without a tenant", []string{a}, "", false},
		{"empty configured entry does not match an empty tenant", []string{""}, "", false},
		{"no tenant configured", nil, a, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SendsTo(tc.tenants, tc.tenantID); got != tc.want {
				t.Fatalf("SendsTo(%v, %q) = %v, want %v", tc.tenants, tc.tenantID, got, tc.want)
			}
		})
	}
}
