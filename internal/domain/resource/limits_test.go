package resource_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
)

func TestMerge_ZeroInherit(t *testing.T) {
	base := resource.Limits{MemoryMB: 512, CPUQuota: 1000, PidsLimit: 100, StorageGB: 10, NetworkMode: "none"}
	override := resource.Limits{} // all zero

	result := resource.Merge(base, override)
	if result != base {
		t.Fatalf("expected base unchanged, got %+v", result)
	}
}

func TestMerge_AllOverride(t *testing.T) {
	base := resource.Limits{MemoryMB: 512, CPUQuota: 1000, PidsLimit: 100, StorageGB: 10, NetworkMode: "none"}
	override := resource.Limits{MemoryMB: 1024, CPUQuota: 2000, PidsLimit: 200, StorageGB: 20, NetworkMode: "bridge"}

	result := resource.Merge(base, override)
	if result != override {
		t.Fatalf("expected all overridden, got %+v", result)
	}
}

func TestCap_Enforced(t *testing.T) {
	limits := resource.Limits{MemoryMB: 2048, CPUQuota: 4000, PidsLimit: 500, StorageGB: 50}
	ceiling := resource.Limits{MemoryMB: 1024, CPUQuota: 2000, PidsLimit: 200, StorageGB: 20}

	result := resource.Cap(limits, ceiling)
	if result.MemoryMB != 1024 {
		t.Fatalf("expected MemoryMB capped to 1024, got %d", result.MemoryMB)
	}
	if result.CPUQuota != 2000 {
		t.Fatalf("expected CPUQuota capped to 2000, got %d", result.CPUQuota)
	}
	if result.PidsLimit != 200 {
		t.Fatalf("expected PidsLimit capped to 200, got %d", result.PidsLimit)
	}
	if result.StorageGB != 20 {
		t.Fatalf("expected StorageGB capped to 20, got %d", result.StorageGB)
	}
}

func TestCap_NoCap(t *testing.T) {
	limits := resource.Limits{MemoryMB: 512, CPUQuota: 1000, PidsLimit: 100, StorageGB: 10}
	ceiling := resource.Limits{MemoryMB: 1024, CPUQuota: 2000, PidsLimit: 200, StorageGB: 20}

	result := resource.Cap(limits, ceiling)
	if result != limits {
		t.Fatalf("expected no capping, got %+v", result)
	}
}

func TestMerge_EmptyOverride(t *testing.T) {
	base := resource.Limits{}
	override := resource.Limits{}

	result := resource.Merge(base, override)
	if result != (resource.Limits{}) {
		t.Fatalf("expected zero, got %+v", result)
	}
}
