package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BenchmarkSuiteService manages benchmark suite CRUD and dataset listing.
type BenchmarkSuiteService struct {
	store    database.Store
	datasets operatorDir // benchmark.datasets_dir (KI-107)
}

// NewBenchmarkSuiteService creates a suite service.
func NewBenchmarkSuiteService(store database.Store, datasetsDir string) *BenchmarkSuiteService {
	return &BenchmarkSuiteService{store: store, datasets: newOperatorDir(datasetsDir)}
}

// SeedDefaultSuites creates built-in benchmark suites if they don't exist.
func (s *BenchmarkSuiteService) SeedDefaultSuites(ctx context.Context) {
	existing, err := s.store.ListBenchmarkSuites(ctx)
	if err != nil {
		slog.Warn("failed to list suites for seeding", "error", err)
		return
	}
	seen := make(map[string]bool, len(existing))
	for i := range existing {
		seen[existing[i].ProviderName] = true
	}
	for i := range defaultSuites {
		def := defaultSuites[i]
		if seen[def.ProviderName] {
			continue
		}
		if _, err := s.RegisterSuite(ctx, &def); err != nil {
			slog.Warn("failed to seed benchmark suite", "name", def.Name, "error", err)
		} else {
			slog.Info("seeded benchmark suite", "name", def.Name, "provider", def.ProviderName)
		}
	}
}

// RegisterSuite validates and persists a new benchmark suite.
// If Type is empty, it is auto-derived from ProviderName.
func (s *BenchmarkSuiteService) RegisterSuite(ctx context.Context, req *benchmark.CreateSuiteRequest) (*benchmark.Suite, error) {
	if req.Type == "" {
		req.Type = benchmark.ProviderDefaultType(req.ProviderName)
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	suite := &benchmark.Suite{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Description:  req.Description,
		Type:         req.Type,
		ProviderName: req.ProviderName,
		Config:       req.Config,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.CreateBenchmarkSuite(ctx, suite); err != nil {
		return nil, err
	}
	return suite, nil
}

// GetSuite retrieves a benchmark suite by ID.
func (s *BenchmarkSuiteService) GetSuite(ctx context.Context, id string) (*benchmark.Suite, error) {
	return s.store.GetBenchmarkSuite(ctx, id)
}

// ListSuites returns all registered benchmark suites.
func (s *BenchmarkSuiteService) ListSuites(ctx context.Context) ([]benchmark.Suite, error) {
	return s.store.ListBenchmarkSuites(ctx)
}

// UpdateSuite updates an existing benchmark suite.
func (s *BenchmarkSuiteService) UpdateSuite(ctx context.Context, suite *benchmark.Suite) error {
	return s.store.UpdateBenchmarkSuite(ctx, suite)
}

// DeleteSuite removes a benchmark suite by ID.
func (s *BenchmarkSuiteService) DeleteSuite(ctx context.Context, id string) error {
	return s.store.DeleteBenchmarkSuite(ctx, id)
}

// ListDatasets scans the datasets directory for YAML files and returns metadata.
func (s *BenchmarkSuiteService) ListDatasets() ([]benchmark.DatasetInfo, error) {
	root, err := s.datasets.open()
	if errors.Is(err, errNoOperatorDir) || errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open datasets dir: %w", err)
	}
	defer func() { _ = root.Close() }()

	// Walked and read inside the directory (KI-107): a symlink out of it is
	// neither followed nor read.
	var datasets []benchmark.DatasetInfo
	err = root.WalkDir(".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(path.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, _, err := root.ReadFile(name, maxDatasetFileSize)
		if err != nil {
			return nil // skip unreadable files and symlinks out of the directory
		}
		var df datasetFile
		if err := yaml.Unmarshal(data, &df); err != nil {
			return nil // skip invalid files
		}

		datasets = append(datasets, benchmark.DatasetInfo{
			Name:        df.Name,
			Description: df.Description,
			TaskCount:   len(df.Tasks),
			Path:        filepath.FromSlash(name),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk datasets dir: %w", err)
	}
	return datasets, nil
}
