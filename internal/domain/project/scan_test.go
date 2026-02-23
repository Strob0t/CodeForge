package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanWorkspace(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string // filename → content
		wantLangs      []string
		wantFrameworks map[string][]string // lang → frameworks
		wantErr        bool
	}{
		{
			name: "go project with chi",
			files: map[string]string{
				"go.mod": "module example.com/foo\n\nrequire github.com/go-chi/chi/v5 v5.0.0\n",
				"go.sum": "",
			},
			wantLangs:      []string{"go"},
			wantFrameworks: map[string][]string{"go": {"chi"}},
		},
		{
			name: "typescript project with solidjs",
			files: map[string]string{
				"package.json":  `{"dependencies": {"solid-js": "^1.0.0"}}`,
				"tsconfig.json": `{"compilerOptions": {}}`,
			},
			wantLangs:      []string{"typescript"},
			wantFrameworks: map[string][]string{"typescript": {"solidjs"}},
		},
		{
			name: "javascript project without tsconfig",
			files: map[string]string{
				"package.json": `{"dependencies": {"express": "^4.0.0"}}`,
			},
			wantLangs:      []string{"javascript"},
			wantFrameworks: map[string][]string{"javascript": {"express"}},
		},
		{
			name: "python project",
			files: map[string]string{
				"pyproject.toml": "[project]\ndependencies = [\"fastapi\"]\n",
			},
			wantLangs:      []string{"python"},
			wantFrameworks: map[string][]string{"python": {"fastapi"}},
		},
		{
			name: "rust project",
			files: map[string]string{
				"Cargo.toml": "[dependencies]\naxum = \"0.6\"\n",
			},
			wantLangs:      []string{"rust"},
			wantFrameworks: map[string][]string{"rust": {"axum"}},
		},
		{
			name: "multi-language project",
			files: map[string]string{
				"go.mod":       "module example.com/foo\n",
				"package.json": `{"dependencies": {"react": "^18.0.0"}}`,
			},
			wantLangs: []string{"go", "javascript"},
		},
		{
			name:      "empty directory",
			files:     map[string]string{},
			wantLangs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			result, err := ScanWorkspace(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check detected languages.
			gotLangs := make(map[string]bool)
			for _, l := range result.Languages {
				gotLangs[l.Name] = true
			}
			for _, want := range tt.wantLangs {
				if !gotLangs[want] {
					t.Errorf("expected language %q not found in %v", want, result.Languages)
				}
			}
			if len(result.Languages) != len(tt.wantLangs) {
				t.Errorf("language count: got %d, want %d", len(result.Languages), len(tt.wantLangs))
			}

			// Check frameworks if specified.
			for _, l := range result.Languages {
				wantFw := tt.wantFrameworks[l.Name]
				if len(wantFw) == 0 {
					continue
				}
				gotFw := make(map[string]bool)
				for _, fw := range l.Frameworks {
					gotFw[fw] = true
				}
				for _, fw := range wantFw {
					if !gotFw[fw] {
						t.Errorf("language %q: expected framework %q not found in %v", l.Name, fw, l.Frameworks)
					}
				}
			}

			// Recommendations should be non-empty for any detected language.
			if len(result.Languages) > 0 && len(result.Recommendations) == 0 {
				t.Error("expected recommendations for detected languages, got none")
			}
		})
	}
}

func TestScanWorkspace_NonexistentPath(t *testing.T) {
	_, err := ScanWorkspace("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

func TestScanWorkspace_NotADirectory(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, err = ScanWorkspace(f.Name())
	if err == nil {
		t.Fatal("expected error for non-directory path, got nil")
	}
}
