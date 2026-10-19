package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// datasetLayout builds a datasets directory with basic.yaml and sub/more.yml,
// an outside directory with a dataset, and symlinks in the datasets directory
// leading out.
func datasetLayout(t *testing.T) (dir, outside string) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "datasets")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(dir, "sub"), outside} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	const dataset = "name: %s\ntasks:\n  - id: t1\n    name: T\n    input: x\n"
	for path, name := range map[string]string{
		filepath.Join(dir, "basic.yaml"):      "basic",
		filepath.Join(dir, "sub", "more.yml"): "more",
		filepath.Join(outside, "secret.yaml"): "outside-secret",
	} {
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(dataset, "%s", name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"abs.yaml": filepath.Join(outside, "secret.yaml"),
		"rel.yaml": "../outside/secret.yaml",
		"out":      outside,
		"in.yaml":  "basic.yaml",
	} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	return dir, outside
}

func startDatasetRun(t *testing.T, dir, dataset string) (*benchmark.Run, *benchMockQueue, *benchMockStore, error) {
	t.Helper()
	store := newBenchMockStore()
	svc := newTestBenchmarkServiceWithDir(store, dir)
	q := &benchMockQueue{}
	svc.SetQueue(q)
	run, err := svc.StartRun(context.Background(), &benchmark.CreateRunRequest{
		Dataset: dataset, Model: "gpt-4", Metrics: []string{"correctness"},
	})
	return run, q, store, err
}

func TestStartRun_DatasetsStayInsideTheDatasetsDirectory(t *testing.T) {
	dir, outside := datasetLayout(t)

	for dataset, want := range map[string]string{
		"basic":                          filepath.Join(dir, "basic.yaml"),
		"basic.yaml":                     filepath.Join(dir, "basic.yaml"),
		"sub/more.yml":                   filepath.Join(dir, "sub", "more.yml"),
		"in":                             filepath.Join(dir, "in.yaml"),
		filepath.Join(dir, "basic.yaml"): filepath.Join(dir, "basic.yaml"),
	} {
		_, q, _, err := startDatasetRun(t, dir, dataset)
		if err != nil {
			t.Fatalf("StartRun(%q): %v", dataset, err)
		}
		var payload messagequeue.BenchmarkRunRequestPayload
		if len(q.published) != 1 || json.Unmarshal(q.published[0].Data, &payload) != nil {
			t.Fatalf("StartRun(%q) published %d messages", dataset, len(q.published))
		}
		if payload.DatasetPath != want {
			t.Errorf("StartRun(%q) dataset_path = %q, want %q", dataset, payload.DatasetPath, want)
		}
	}

	// Refused before the run is stored: absolute paths outside and names
	// that climb out.
	for _, dataset := range []string{
		filepath.Join(outside, "secret.yaml"), "/etc/passwd", "../outside/secret", "sub/../../outside/secret.yaml",
	} {
		run, q, store, err := startDatasetRun(t, dir, dataset)
		if !errors.Is(err, domain.ErrValidation) || run != nil {
			t.Errorf("StartRun(%q) = %v, %v; want a validation error", dataset, run, err)
		}
		if err != nil && !strings.Contains(err.Error(), "datasets directory") {
			t.Errorf("StartRun(%q) error %q does not name the datasets directory", dataset, err)
		}
		if len(q.published) != 0 || len(store.benchRuns) != 0 {
			t.Errorf("StartRun(%q): %d published, %d stored; want none", dataset, len(q.published), len(store.benchRuns))
		}
	}

	// Symlinks out of the datasets directory are refused when resolved.
	for _, dataset := range []string{"abs", "rel.yaml", "out/secret.yaml"} {
		run, q, _, err := startDatasetRun(t, dir, dataset)
		if !errors.Is(err, domain.ErrValidation) || run != nil {
			t.Errorf("StartRun(%q) = %v, %v; want a validation error", dataset, run, err)
		}
		if len(q.published) != 0 {
			t.Errorf("StartRun(%q) published %d messages, want none", dataset, len(q.published))
		}
	}
}

func TestStartRun_WithoutADatasetsDirectoryOnlyNamesPass(t *testing.T) {
	_, _, _, err := startDatasetRun(t, "", "/etc/passwd")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("absolute dataset without a datasets directory: %v, want a validation error", err)
	}
}

func TestListDatasets_StaysInsideTheDatasetsDirectory(t *testing.T) {
	dir, _ := datasetLayout(t)
	svc := newTestBenchmarkServiceWithDir(newBenchMockStore(), dir)
	datasets, err := svc.ListDatasets()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range datasets {
		names = append(names, d.Name+"@"+d.Path)
	}
	got := strings.Join(names, ",")
	if strings.Contains(got, "outside-secret") {
		t.Fatalf("ListDatasets read outside the datasets directory: %s", got)
	}
	for _, want := range []string{"basic@basic.yaml", "more@" + filepath.Join("sub", "more.yml"), "basic@in.yaml"} {
		if !strings.Contains(got, want) {
			t.Errorf("ListDatasets = %s, want %s", got, want)
		}
	}
}
