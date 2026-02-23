package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
)

// Sandbox represents a running Docker container for agent execution.
type Sandbox struct {
	ContainerID string `json:"container_id"`
	RunID       string `json:"run_id"`
	WorkDir     string `json:"work_dir"`
	Status      string `json:"status"`
}

// SandboxConfig holds default sandbox resource limits.
type SandboxConfig struct {
	MemoryMB    int    `yaml:"memory_mb"`
	CPUQuota    int    `yaml:"cpu_quota"`
	PidsLimit   int    `yaml:"pids_limit"`
	StorageGB   int    `yaml:"storage_gb"`
	NetworkMode string `yaml:"network_mode"`
	Image       string `yaml:"image"`
}

// DefaultSandboxConfig returns sensible defaults.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MemoryMB:    512,
		CPUQuota:    1000,
		PidsLimit:   100,
		StorageGB:   10,
		NetworkMode: "none",
		Image:       "ubuntu:22.04",
	}
}

// SandboxService manages Docker containers for sandboxed agent execution.
type SandboxService struct {
	defaults  SandboxConfig
	mu        sync.Mutex
	sandboxes map[string]*Sandbox
}

// NewSandboxService creates a SandboxService with the given defaults.
func NewSandboxService(cfg SandboxConfig) *SandboxService {
	return &SandboxService{
		defaults:  cfg,
		sandboxes: make(map[string]*Sandbox),
	}
}

// Create creates a Docker container for the given run.
func (s *SandboxService) Create(ctx context.Context, runID, workspacePath string, overrides ...resource.Limits) (*Sandbox, error) {
	// Start from config defaults
	limits := resource.Limits{
		MemoryMB:    s.defaults.MemoryMB,
		CPUQuota:    s.defaults.CPUQuota,
		PidsLimit:   s.defaults.PidsLimit,
		StorageGB:   s.defaults.StorageGB,
		NetworkMode: s.defaults.NetworkMode,
	}

	// Apply overrides in order (policy, then agent)
	for _, o := range overrides {
		limits = resource.Merge(limits, o)
	}

	// Cap at global max (defaults act as max)
	maxLimits := resource.Limits{
		MemoryMB:  s.defaults.MemoryMB * 4, // 4x default as hard cap
		CPUQuota:  s.defaults.CPUQuota * 4,
		PidsLimit: s.defaults.PidsLimit * 4,
		StorageGB: s.defaults.StorageGB * 4,
	}
	limits = resource.Cap(limits, maxLimits)

	containerName := fmt.Sprintf("codeforge-%s", shortID(runID))
	image := s.defaults.Image

	args := []string{
		"create",
		"--name", containerName,
		fmt.Sprintf("--memory=%dm", limits.MemoryMB),
		fmt.Sprintf("--cpus=%d", limits.CPUQuota/1000),
		fmt.Sprintf("--pids-limit=%d", limits.PidsLimit),
	}

	if limits.NetworkMode != "" {
		args = append(args, fmt.Sprintf("--network=%s", limits.NetworkMode))
	}

	args = append(args,
		"-v", fmt.Sprintf("%s:/workspace", workspacePath),
		"--read-only",
		"--tmpfs", "/tmp",
		"--security-opt=no-new-privileges",
		"--cap-drop=ALL",
		image,
	)

	output, err := runDocker(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("sandbox create: %w", err)
	}

	sb := &Sandbox{
		ContainerID: strings.TrimSpace(output),
		RunID:       runID,
		WorkDir:     workspacePath,
		Status:      "created",
	}

	s.mu.Lock()
	s.sandboxes[runID] = sb
	s.mu.Unlock()

	return sb, nil
}

// CreateHybrid creates a Docker container for hybrid execution mode.
// Unlike Create, the workspace is mounted read-write and the filesystem is not read-only,
// allowing the agent to modify source files directly while commands execute inside the container.
func (s *SandboxService) CreateHybrid(ctx context.Context, runID, workspacePath string, overrides ...resource.Limits) (*Sandbox, error) {
	limits := resource.Limits{
		MemoryMB:    s.defaults.MemoryMB,
		CPUQuota:    s.defaults.CPUQuota,
		PidsLimit:   s.defaults.PidsLimit,
		StorageGB:   s.defaults.StorageGB,
		NetworkMode: s.defaults.NetworkMode,
	}

	for _, o := range overrides {
		limits = resource.Merge(limits, o)
	}

	maxLimits := resource.Limits{
		MemoryMB:  s.defaults.MemoryMB * 4,
		CPUQuota:  s.defaults.CPUQuota * 4,
		PidsLimit: s.defaults.PidsLimit * 4,
		StorageGB: s.defaults.StorageGB * 4,
	}
	limits = resource.Cap(limits, maxLimits)

	containerName := fmt.Sprintf("codeforge-hybrid-%s", shortID(runID))
	image := s.defaults.Image

	args := []string{
		"create",
		"--name", containerName,
		fmt.Sprintf("--memory=%dm", limits.MemoryMB),
		fmt.Sprintf("--cpus=%d", limits.CPUQuota/1000),
		fmt.Sprintf("--pids-limit=%d", limits.PidsLimit),
	}

	if limits.NetworkMode != "" {
		args = append(args, fmt.Sprintf("--network=%s", limits.NetworkMode))
	}

	// Hybrid mode: mount read-write, no --read-only flag
	args = append(args,
		"-v", fmt.Sprintf("%s:/workspace", workspacePath),
		"--tmpfs", "/tmp",
		image,
		"sleep", "infinity", // Keep container running for docker exec
	)

	output, err := runDocker(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("hybrid sandbox create: %w", err)
	}

	sb := &Sandbox{
		ContainerID: strings.TrimSpace(output),
		RunID:       runID,
		WorkDir:     workspacePath,
		Status:      "created",
	}

	s.mu.Lock()
	s.sandboxes[runID] = sb
	s.mu.Unlock()

	return sb, nil
}

// Start starts the container for the given run.
func (s *SandboxService) Start(ctx context.Context, runID string) error {
	s.mu.Lock()
	sb, ok := s.sandboxes[runID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("sandbox not found for run %s", runID)
	}

	if _, err := runDocker(ctx, "start", sb.ContainerID); err != nil {
		return fmt.Errorf("sandbox start: %w", err)
	}

	s.mu.Lock()
	sb.Status = "running"
	s.mu.Unlock()

	return nil
}

// Exec runs a command inside the sandbox container.
func (s *SandboxService) Exec(ctx context.Context, runID string, command []string) (stdout, stderr string, err error) {
	s.mu.Lock()
	sb, ok := s.sandboxes[runID]
	s.mu.Unlock()

	if !ok {
		return "", "", fmt.Errorf("sandbox not found for run %s", runID)
	}

	args := append([]string{"exec", sb.ContainerID}, command...)

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: docker args are constructed internally, not from user input
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// Stop stops the container for the given run.
func (s *SandboxService) Stop(ctx context.Context, runID string) error {
	s.mu.Lock()
	sb, ok := s.sandboxes[runID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("sandbox not found for run %s", runID)
	}

	if _, err := runDocker(ctx, "stop", "-t", "10", sb.ContainerID); err != nil {
		return fmt.Errorf("sandbox stop: %w", err)
	}

	s.mu.Lock()
	sb.Status = "stopped"
	s.mu.Unlock()

	return nil
}

// Remove removes the container for the given run.
func (s *SandboxService) Remove(ctx context.Context, runID string) error {
	s.mu.Lock()
	sb, ok := s.sandboxes[runID]
	s.mu.Unlock()

	if !ok {
		// Already removed or never created — not an error.
		return nil
	}

	if _, err := runDocker(ctx, "rm", "-f", sb.ContainerID); err != nil {
		return fmt.Errorf("sandbox remove: %w", err)
	}

	s.mu.Lock()
	delete(s.sandboxes, runID)
	s.mu.Unlock()

	return nil
}

// Get returns the sandbox for the given run.
func (s *SandboxService) Get(runID string) (*Sandbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[runID]
	return sb, ok
}

// shortID returns the first 12 characters of an ID (or the full string if shorter).
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// runDocker executes a docker command and returns stdout.
func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: docker args are constructed internally, not from user input

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
