package config

import (
	"math"
	"strings"
	"testing"
	"time"
)

// KI-91 review: a held message expires quarantine.expiry_hours after it was
// held. 0 or a negative value made every held message overdue at once (an
// approval always answered 409 and the expiry sweep rejected everything), and
// a value whose hours overflow a time.Duration wrapped into the past: both
// are refused at startup.
func TestValidate_QuarantineExpiryHours(t *testing.T) {
	maxHours := int(math.MaxInt64 / int64(time.Hour))
	tests := []struct {
		name    string
		hours   int
		wantErr bool
	}{
		{"default", Defaults().Quarantine.ExpiryHours, false},
		{"one hour", 1, false},
		{"largest duration", maxHours, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"overflows a duration", maxHours + 1, true},
		{"max int", math.MaxInt, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.Quarantine.ExpiryHours = tt.hours
			err := validate(&cfg)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "quarantine.expiry_hours") {
					t.Fatalf("validate() = %v, want an error naming quarantine.expiry_hours", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
			if got := cfg.Quarantine.Expiry(); got != time.Duration(tt.hours)*time.Hour || got <= 0 {
				t.Fatalf("Expiry() = %v, want %d hours", got, tt.hours)
			}
		})
	}
}
