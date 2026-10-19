package policy

import "testing"

func mustGlob(t *testing.T, pattern, value string) bool {
	t.Helper()
	matched, err := matchGlob(pattern, value)
	if err != nil {
		t.Fatalf("matchGlob(%q, %q): %v", pattern, value, err)
	}
	return matched
}

func TestMatchGlobExact(t *testing.T) {
	if !mustGlob(t, ".env", ".env") {
		t.Error("expected .env to match .env")
	}
	if mustGlob(t, ".env", ".env.local") {
		t.Error("expected .env not to match .env.local")
	}
}

func TestMatchGlobStar(t *testing.T) {
	if !mustGlob(t, "*.go", "main.go") {
		t.Error("expected *.go to match main.go")
	}
	if mustGlob(t, "*.go", "src/main.go") {
		t.Error("expected *.go not to match src/main.go (single *)")
	}
}

func TestMatchGlobDoubleStar(t *testing.T) {
	if !mustGlob(t, "**/*.go", "src/main.go") {
		t.Error("expected **/*.go to match src/main.go")
	}
	if !mustGlob(t, "**/*.go", "internal/service/policy.go") {
		t.Error("expected **/*.go to match internal/service/policy.go")
	}
	if !mustGlob(t, "secrets/**", "secrets/api.key") {
		t.Error("expected secrets/** to match secrets/api.key")
	}
	if !mustGlob(t, "secrets/**", "secrets/nested/deep.key") {
		t.Error("expected secrets/** to match secrets/nested/deep.key")
	}
	if mustGlob(t, "secrets/**", "other/file.txt") {
		t.Error("expected secrets/** not to match other/file.txt")
	}
}

func TestMatchGlobNoMatch(t *testing.T) {
	if mustGlob(t, "*.ts", "main.go") {
		t.Error("expected *.ts not to match main.go")
	}
}

func TestMatchGlobDoubleStarEnv(t *testing.T) {
	if !mustGlob(t, "**/.env", "src/.env") {
		t.Error("expected **/.env to match src/.env")
	}
	if !mustGlob(t, "**/.env", "deep/nested/.env") {
		t.Error("expected **/.env to match deep/nested/.env")
	}
	if !mustGlob(t, "**/.env", ".env") {
		t.Error("expected **/.env to match .env (zero segments)")
	}
}

func TestMatchGlobMalformed(t *testing.T) {
	for _, pattern := range []string{"secrets/[", "**/[a-"} {
		if _, err := matchGlob(pattern, "secrets/x"); err == nil {
			t.Errorf("matchGlob(%q) returned no error", pattern)
		}
	}
}

func TestMatchesAnyGlob(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		value    string
		deny     bool
		want     bool
	}{
		{"allow exact", []string{"src/**"}, "src/a.go", false, true},
		{"allow is case sensitive", []string{"src/**"}, "SRC/a.go", false, false},
		{"deny ignores case", []string{".env"}, ".ENV", true, true},
		{"deny pattern case ignored", []string{"**/Secrets/**"}, "a/secrets/b", true, true},
		{"deny malformed pattern matches", []string{"x/["}, "src/a.go", true, true},
		{"allow malformed pattern does not match", []string{"x/["}, "x/a", false, false},
		{"no patterns", nil, "a", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesAnyGlob(tt.patterns, tt.value, tt.deny); got != tt.want {
				t.Errorf("matchesAnyGlob(%q, %q, deny=%v) = %v, want %v", tt.patterns, tt.value, tt.deny, got, tt.want)
			}
		})
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		fold    bool
		want    bool
	}{
		{"git *", "git status", false, true},
		{"git *", "git", false, false},
		{"git*", "git status", false, true},
		{"git*", "gitk", false, true},
		{"*", "", false, true},
		{"", "", false, true},
		{"", "x", false, false},
		{"g?t *", "got it", false, true},
		{"*push*", "git push origin", false, true},
		{"a*b*c", "axxbyyc", false, true},
		{"a*b*c", "axxbyy", false, false},
		{"npm run *", "npm run build -- --watch /tmp", false, true},
		{"RM *", "rm -rf /", true, true},
		{"RM *", "rm -rf /", false, false},
	}
	for _, tt := range tests {
		if got := matchWildcard(tt.pattern, tt.s, tt.fold); got != tt.want {
			t.Errorf("matchWildcard(%q, %q, %v) = %v, want %v", tt.pattern, tt.s, tt.fold, got, tt.want)
		}
	}
}
