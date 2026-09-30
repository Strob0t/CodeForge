package config

import (
	"slices"
	"strings"
	"testing"
)

// S3 follow-up 1f: workspace.adopt_roots lists absolute directories admins
// may adopt from; default none.
func TestWorkspaceAdoptRoots(t *testing.T) {
	if roots := Defaults().Workspace.AdoptRoots; len(roots) != 0 {
		t.Fatalf("default adopt roots = %v, want none", roots)
	}

	t.Setenv("CODEFORGE_WORKSPACE_ADOPT_ROOTS", "/srv/e2e, /tmp/codeforge-e2e")
	cfg := Defaults()
	loadEnv(&cfg)
	if want := []string{"/srv/e2e", "/tmp/codeforge-e2e"}; !slices.Equal(cfg.Workspace.AdoptRoots, want) {
		t.Fatalf("adopt roots from env = %v, want %v", cfg.Workspace.AdoptRoots, want)
	}

	for _, bad := range []string{"relative/dir", "/", ""} {
		cfg := Defaults()
		cfg.AppEnv = "development"
		if err := ensureSecrets(&cfg); err != nil {
			t.Fatal(err)
		}
		cfg.Workspace.AdoptRoots = []string{bad}
		if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "workspace.adopt_roots") {
			t.Errorf("adopt root %q: validate = %v, want an error", bad, err)
		}
	}
}
