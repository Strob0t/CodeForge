package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BenchmarkService manages benchmark runs and results.
type BenchmarkService struct {
	store       database.Store
	datasetsDir string
}

// NewBenchmarkService creates a benchmark service.
func NewBenchmarkService(store database.Store, datasetsDir string) *BenchmarkService {
	return &BenchmarkService{store: store, datasetsDir: datasetsDir}
}

// CreateRun validates and persists a new benchmark run.
func (s *BenchmarkService) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r := &benchmark.Run{
		ID:        uuid.New().String(),
		Dataset:   req.Dataset,
		Model:     req.Model,
		Metrics:   req.Metrics,
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateBenchmarkRun(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// GetRun retrieves a benchmark run by ID.
func (s *BenchmarkService) GetRun(ctx context.Context, id string) (*benchmark.Run, error) {
	return s.store.GetBenchmarkRun(ctx, id)
}

// ListRuns returns all benchmark runs.
func (s *BenchmarkService) ListRuns(ctx context.Context) ([]benchmark.Run, error) {
	return s.store.ListBenchmarkRuns(ctx)
}

// UpdateRun updates a benchmark run.
func (s *BenchmarkService) UpdateRun(ctx context.Context, r *benchmark.Run) error {
	return s.store.UpdateBenchmarkRun(ctx, r)
}

// DeleteRun deletes a benchmark run and its results.
func (s *BenchmarkService) DeleteRun(ctx context.Context, id string) error {
	return s.store.DeleteBenchmarkRun(ctx, id)
}

// ListResults returns all results for a benchmark run.
func (s *BenchmarkService) ListResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	return s.store.ListBenchmarkResults(ctx, runID)
}

// Compare loads two runs and their results for side-by-side comparison.
func (s *BenchmarkService) Compare(ctx context.Context, idA, idB string) (*benchmark.CompareResult, error) {
	runA, err := s.store.GetBenchmarkRun(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("run A: %w", err)
	}
	runB, err := s.store.GetBenchmarkRun(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("run B: %w", err)
	}
	resultsA, err := s.store.ListBenchmarkResults(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("results A: %w", err)
	}
	resultsB, err := s.store.ListBenchmarkResults(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("results B: %w", err)
	}
	return &benchmark.CompareResult{
		RunA:    runA,
		RunB:    runB,
		ResultA: resultsA,
		ResultB: resultsB,
	}, nil
}

// datasetFile is the YAML structure of a benchmark dataset file.
type datasetFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tasks       []struct {
		ID             string `yaml:"id"`
		Name           string `yaml:"name"`
		Input          string `yaml:"input"`
		ExpectedOutput string `yaml:"expected_output"`
	} `yaml:"tasks"`
}

// ListDatasets scans the datasets directory for YAML files and returns metadata.
func (s *BenchmarkService) ListDatasets() ([]benchmark.DatasetInfo, error) {
	if s.datasetsDir == "" {
		return nil, nil
	}

	var datasets []benchmark.DatasetInfo
	err := filepath.WalkDir(s.datasetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // path is from WalkDir within datasetsDir
		if err != nil {
			return nil // skip unreadable files
		}
		var df datasetFile
		if err := yaml.Unmarshal(data, &df); err != nil {
			return nil // skip invalid files
		}

		rel, _ := filepath.Rel(s.datasetsDir, path)
		datasets = append(datasets, benchmark.DatasetInfo{
			Name:        df.Name,
			Description: df.Description,
			TaskCount:   len(df.Tasks),
			Path:        rel,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk datasets dir: %w", err)
	}
	return datasets, nil
}
