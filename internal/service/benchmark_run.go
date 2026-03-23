package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

// CreateRun validates and persists a new benchmark run.
func (m *BenchmarkRunManager) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, err
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
		Dataset:            req.Dataset,
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
	datasetPath := m.resolveDatasetPath(ctx, run)
	if datasetPath == "" {
		return nil, fmt.Errorf("%w: dataset %q not found", domain.ErrValidation, run.Dataset)
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
		TenantID:           tenantctx.FromContext(ctx),
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

// resolveDatasetPath resolves a dataset name to an absolute file path.
// Returns the resolved path, or "" if the dataset cannot be found and no suite fallback exists.
func (m *BenchmarkRunManager) resolveDatasetPath(ctx context.Context, run *benchmark.Run) string {
	datasetPath := run.Dataset
	if m.suiteSvc == nil || m.suiteSvc.datasetsDir == "" || filepath.IsAbs(datasetPath) {
		return datasetPath
	}

	base := datasetPath
	if !strings.HasSuffix(base, ".yaml") {
		base += ".yaml"
	}
	candidate := filepath.Join(m.suiteSvc.datasetsDir, base)
	absCandidate, _ := filepath.Abs(candidate)
	if _, statErr := os.Stat(absCandidate); statErr == nil {
		slog.Info("resolved dataset path", "original", run.Dataset, "resolved", absCandidate)
		return absCandidate
	}

	if run.SuiteID == "" {
		// No suite fallback -- the dataset is mandatory and must exist.
		run.Status = benchmark.StatusFailed
		run.ErrorMessage = fmt.Sprintf("dataset %q not found", run.Dataset)
		logBestEffort(ctx, m.store.UpdateBenchmarkRun(ctx, run), "UpdateBenchmarkRun", slog.String("run_id", run.ID))
		return ""
	}

	slog.Warn("dataset path resolution failed, relying on suite provider",
		"original", run.Dataset, "candidate", absCandidate)
	return datasetPath
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
