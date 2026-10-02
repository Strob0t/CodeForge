package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// BenchmarkRunManager handles benchmark run lifecycle (create, start, list, update, delete).
type BenchmarkRunManager struct {
	store      database.Store
	queue      messagequeue.Queue
	routingSvc *RoutingService
	suiteSvc   *BenchmarkSuiteService
}

// NewBenchmarkRunManager creates a run manager.
func NewBenchmarkRunManager(store database.Store, suiteSvc *BenchmarkSuiteService) *BenchmarkRunManager {
	return &BenchmarkRunManager{store: store, suiteSvc: suiteSvc}
}

// SetQueue sets the NATS queue for publishing benchmark requests.
func (m *BenchmarkRunManager) SetQueue(q messagequeue.Queue) { m.queue = q }

// SetRoutingService sets the routing service for benchmark -> routing integration.
func (m *BenchmarkRunManager) SetRoutingService(routingSvc *RoutingService) {
	m.routingSvc = routingSvc
}

// maxDatasetFileSize caps a dataset file the Go Core reads.
const maxDatasetFileSize = 10 << 20

// datasets returns the benchmark datasets directory (zero when not configured).
func (m *BenchmarkRunManager) datasets() operatorDir {
	if m.suiteSvc == nil {
		return operatorDir{}
	}
	return m.suiteSvc.datasets
}

// CreateRun validates and persists a new benchmark run. The dataset is a
// name or a path inside the benchmark datasets directory (KI-107), stored
// relative to it.
func (m *BenchmarkRunManager) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := checkBenchmarkExecMode(req.ExecMode); err != nil {
		return nil, err
	}
	dataset := req.Dataset
	if dataset != "" {
		rel, ok := m.datasets().relative(dataset)
		if !ok {
			return nil, fmt.Errorf("%w: dataset %q must be a dataset name or a path inside the benchmark datasets directory (benchmark.datasets_dir)",
				domain.ErrValidation, dataset)
		}
		dataset = rel
	}
	rolloutCount := req.RolloutCount
	if rolloutCount < 1 {
		rolloutCount = 1
	}
	rolloutStrategy := req.RolloutStrategy
	if rolloutStrategy == "" {
		rolloutStrategy = "best"
	}
	r := &benchmark.Run{
		ID:                 uuid.New().String(),
		Dataset:            dataset,
		Model:              req.Model,
		Metrics:            req.Metrics,
		Status:             benchmark.StatusRunning,
		SuiteID:            req.SuiteID,
		BenchmarkType:      req.BenchmarkType,
		ExecMode:           req.ExecMode,
		HybridVerification: req.HybridVerification,
		RolloutCount:       rolloutCount,
		RolloutStrategy:    rolloutStrategy,
		CreatedAt:          time.Now().UTC(),
	}
	if err := m.store.CreateBenchmarkRun(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// StartRun creates a benchmark run in the database and publishes it to NATS
// for Python worker execution. Falls back to CreateRun (DB-only) if queue is nil.
func (m *BenchmarkRunManager) StartRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	run, err := m.CreateRun(ctx, req)
	if err != nil {
		return nil, err
	}

	if m.queue == nil {
		slog.Warn("benchmark NATS queue not configured, run will stay in running state", "run_id", run.ID)
		return run, nil
	}

	// Resolve dataset name to absolute file path.
	datasetPath, err := m.resolveDatasetPath(ctx, run)
	if err != nil {
		return nil, err
	}

	// Resolve provider info from suite (if suite-based run).
	var providerName string
	var providerConfig json.RawMessage
	if run.SuiteID != "" {
		suite, sErr := m.store.GetBenchmarkSuite(ctx, run.SuiteID)
		if sErr != nil {
			slog.Warn("failed to load suite for run, falling back to dataset path", "suite_id", run.SuiteID, "error", sErr)
		} else {
			providerName = suite.ProviderName
			providerConfig = mergeProviderConfig(suite.Config, req.ProviderConfig)
			if run.BenchmarkType == "" {
				run.BenchmarkType = suite.Type
			}
		}
	}

	payload := messagequeue.BenchmarkRunRequestPayload{
		RunID:              run.ID,
		TenantID:           outgoingTenant(ctx, "benchmark.run.request"),
		DatasetPath:        datasetPath,
		Model:              run.Model,
		Metrics:            run.Metrics,
		BenchmarkType:      string(run.BenchmarkType),
		SuiteID:            run.SuiteID,
		ExecMode:           string(run.ExecMode),
		Evaluators:         run.Metrics, // metrics double as evaluator names
		HybridVerification: run.HybridVerification,
		RolloutCount:       run.RolloutCount,
		RolloutStrategy:    run.RolloutStrategy,
		ProviderName:       providerName,
		ProviderConfig:     providerConfig,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark run request: %w", err)
	}

	if err := m.queue.Publish(ctx, messagequeue.SubjectBenchmarkRunRequest, data); err != nil {
		// Run is already saved -- mark as failed if we can't dispatch.
		slog.Error("failed to publish benchmark run request", "run_id", run.ID, "error", err)
		run.Status = benchmark.StatusFailed
		logBestEffort(ctx, m.store.UpdateBenchmarkRun(ctx, run), "UpdateBenchmarkRun", slog.String("run_id", run.ID))
		return nil, fmt.Errorf("publish benchmark run request: %w", err)
	}

	slog.Info("benchmark run dispatched to worker", "run_id", run.ID, "model", run.Model, "dataset", run.Dataset)
	return run, nil
}

// resolveDatasetPath resolves the run's dataset name to the absolute path of
// a regular file inside the datasets directory (".yaml" is added to a name
// without .yaml or .yml), resolved through os.Root (KI-107): a symlink out of
// the directory is refused. A missing dataset fails the run unless a suite
// provider can load the tasks; then the name is passed on. Without a
// datasets directory the name is passed on: the worker reads it below its
// own datasets directory.
func (m *BenchmarkRunManager) resolveDatasetPath(ctx context.Context, run *benchmark.Run) (string, error) {
	dir := m.datasets()
	if run.Dataset == "" || dir.path == "" {
		return run.Dataset, nil
	}

	name := run.Dataset
	if ext := strings.ToLower(path.Ext(name)); ext != ".yaml" && ext != ".yml" {
		name += ".yaml"
	}
	var statErr error
	if root, err := dir.open(); err != nil {
		statErr = err
	} else {
		var info fs.FileInfo
		info, statErr = root.Stat(name)
		_ = root.Close()
		if statErr == nil && info.Mode().IsRegular() {
			slog.Info("resolved dataset path", "original", run.Dataset, "resolved", dir.abs(name))
			return dir.abs(name), nil
		}
	}

	if errors.Is(statErr, workspacefs.ErrLeavesWorkspace) {
		m.failRun(ctx, run, fmt.Sprintf("dataset %q leads out of the benchmark datasets directory", run.Dataset))
		return "", fmt.Errorf("%w: dataset %q leads out of the benchmark datasets directory", domain.ErrValidation, run.Dataset)
	}
	if run.SuiteID == "" {
		// No suite fallback -- the dataset is mandatory and must exist.
		m.failRun(ctx, run, fmt.Sprintf("dataset %q not found", run.Dataset))
		return "", fmt.Errorf("%w: dataset %q not found", domain.ErrValidation, run.Dataset)
	}

	slog.Warn("dataset path resolution failed, relying on suite provider",
		"original", run.Dataset, "error", statErr)
	return run.Dataset, nil
}

// failRun marks a stored run as failed with msg.
func (m *BenchmarkRunManager) failRun(ctx context.Context, run *benchmark.Run, msg string) {
	run.Status = benchmark.StatusFailed
	run.ErrorMessage = msg
	logBestEffort(ctx, m.store.UpdateBenchmarkRun(ctx, run), "UpdateBenchmarkRun", slog.String("run_id", run.ID))
}

// GetRun retrieves a benchmark run by ID.
func (m *BenchmarkRunManager) GetRun(ctx context.Context, id string) (*benchmark.Run, error) {
	return m.store.GetBenchmarkRun(ctx, id)
}

// ListRuns returns all benchmark runs.
func (m *BenchmarkRunManager) ListRuns(ctx context.Context) ([]benchmark.Run, error) {
	return m.store.ListBenchmarkRuns(ctx)
}

// ListRunsFiltered returns benchmark runs matching the given filter.
func (m *BenchmarkRunManager) ListRunsFiltered(ctx context.Context, filter *benchmark.RunFilter) ([]benchmark.Run, error) {
	return m.store.ListBenchmarkRunsFiltered(ctx, filter)
}

// UpdateRun updates a benchmark run. When the run transitions to completed,
// its results are asynchronously seeded into the routing system for MAB learning.
func (m *BenchmarkRunManager) UpdateRun(ctx context.Context, r *benchmark.Run) error {
	if err := m.store.UpdateBenchmarkRun(ctx, r); err != nil {
		return err
	}

	// Seed routing outcomes from completed benchmark runs.
	if r.Status == benchmark.StatusCompleted && m.routingSvc != nil {
		go func() {
			if _, err := m.routingSvc.SeedFromBenchmarkRun(ctx, r.ID); err != nil {
				slog.Warn("seed routing from benchmark run failed", "run_id", r.ID, "error", err)
			}
		}()
	}

	return nil
}

// DeleteRun deletes a benchmark run and its results.
func (m *BenchmarkRunManager) DeleteRun(ctx context.Context, id string) error {
	return m.store.DeleteBenchmarkRun(ctx, id)
}
