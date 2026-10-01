package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The walk for nested .git entries is bounded; reaching a bound refuses the
// workspace (fail closed).
func TestWalkForNestedGit_Bounds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := os.WriteFile(filepath.Join(root, "f"+string(rune('0'+i))), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := walkForNestedGit(root, nil); err != nil {
		t.Fatalf("walk within bounds: %v", err)
	}

	defer func(entries, depth int) { maxNestedWalkEntries, maxNestedWalkDepth = entries, depth }(maxNestedWalkEntries, maxNestedWalkDepth)
	maxNestedWalkEntries = 4
	if err := walkForNestedGit(root, nil); !errors.Is(err, ErrUnsafeRepository) || !strings.Contains(err.Error(), "cannot be checked") {
		t.Fatalf("walk over the entry bound = %v, want a refusal", err)
	}
	maxNestedWalkEntries, maxNestedWalkDepth = 1000, 2
	if err := walkForNestedGit(root, nil); !errors.Is(err, ErrUnsafeRepository) {
		t.Fatalf("walk over the depth bound = %v, want a refusal", err)
	}
	// Ignored directories are not counted.
	maxNestedWalkDepth = 64
	if err := walkForNestedGit(root, map[string]bool{"a/": true}); err != nil {
		t.Fatalf("walk skipping an ignored directory: %v", err)
	}
}
