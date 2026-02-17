package service_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestSandboxConfig_Defaults(t *testing.T) {
	cfg := service.DefaultSandboxConfig()
	if cfg.MemoryMB != 512 {
		t.Fatalf("expected MemoryMB 512, got %d", cfg.MemoryMB)
	}
	if cfg.CPUQuota != 1000 {
		t.Fatalf("expected CPUQuota 1000, got %d", cfg.CPUQuota)
	}
	if cfg.PidsLimit != 100 {
		t.Fatalf("expected PidsLimit 100, got %d", cfg.PidsLimit)
	}
	if cfg.StorageGB != 10 {
		t.Fatalf("expected StorageGB 10, got %d", cfg.StorageGB)
	}
	if cfg.NetworkMode != "none" {
		t.Fatalf("expected NetworkMode none, got %s", cfg.NetworkMode)
	}
	if cfg.Image != "ubuntu:22.04" {
		t.Fatalf("expected Image ubuntu:22.04, got %s", cfg.Image)
	}
}

func TestSandboxService_NewSandboxService(t *testing.T) {
	cfg := service.DefaultSandboxConfig()
	svc := service.NewSandboxService(cfg)
	if svc == nil {
		t.Fatal("expected non-nil SandboxService")
	}
}

func TestSandboxService_GetNotFound(t *testing.T) {
	cfg := service.DefaultSandboxConfig()
	svc := service.NewSandboxService(cfg)
	_, ok := svc.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestResourceLimits_MergeInSandbox(t *testing.T) {
	// Test the merge logic that sandbox uses
	base := resource.Limits{MemoryMB: 512, CPUQuota: 1000, PidsLimit: 100, StorageGB: 10, NetworkMode: "none"}
	policyOverride := resource.Limits{MemoryMB: 1024} // only override memory
	agentOverride := resource.Limits{CPUQuota: 2000}  // only override CPU

	// Simulate sandbox merge: base -> policy -> agent
	merged := resource.Merge(base, policyOverride)
	merged = resource.Merge(merged, agentOverride)

	if merged.MemoryMB != 1024 {
		t.Fatalf("expected MemoryMB 1024 from policy, got %d", merged.MemoryMB)
	}
	if merged.CPUQuota != 2000 {
		t.Fatalf("expected CPUQuota 2000 from agent, got %d", merged.CPUQuota)
	}
	if merged.PidsLimit != 100 {
		t.Fatalf("expected PidsLimit 100 from base, got %d", merged.PidsLimit)
	}
}

func TestResourceLimits_CapInSandbox(t *testing.T) {
	// Simulate: merged limits exceed max -> cap applied
	merged := resource.Limits{MemoryMB: 4096, CPUQuota: 8000, PidsLimit: 500, StorageGB: 50}
	ceiling := resource.Limits{MemoryMB: 2048, CPUQuota: 4000, PidsLimit: 400, StorageGB: 40}

	capped := resource.Cap(merged, ceiling)
	if capped.MemoryMB != 2048 {
		t.Fatalf("expected capped MemoryMB 2048, got %d", capped.MemoryMB)
	}
	if capped.CPUQuota != 4000 {
		t.Fatalf("expected capped CPUQuota 4000, got %d", capped.CPUQuota)
	}
}
