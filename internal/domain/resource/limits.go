// Package resource defines shared resource limit types for agent sandboxes.
package resource

// Limits defines resource constraints for agent execution.
type Limits struct {
	MemoryMB    int    `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	CPUQuota    int    `json:"cpu_quota,omitempty" yaml:"cpu_quota,omitempty"`
	PidsLimit   int    `json:"pids_limit,omitempty" yaml:"pids_limit,omitempty"`
	StorageGB   int    `json:"storage_gb,omitempty" yaml:"storage_gb,omitempty"`
	NetworkMode string `json:"network_mode,omitempty" yaml:"network_mode,omitempty"`
}

// Merge returns a new Limits where non-zero fields from override replace base.
func Merge(base, override Limits) Limits {
	out := base
	if override.MemoryMB > 0 {
		out.MemoryMB = override.MemoryMB
	}
	if override.CPUQuota > 0 {
		out.CPUQuota = override.CPUQuota
	}
	if override.PidsLimit > 0 {
		out.PidsLimit = override.PidsLimit
	}
	if override.StorageGB > 0 {
		out.StorageGB = override.StorageGB
	}
	if override.NetworkMode != "" {
		out.NetworkMode = override.NetworkMode
	}
	return out
}

// Cap returns a new Limits where each field is capped at the corresponding ceiling value.
// A zero ceiling field means no cap for that field.
func Cap(limits, ceiling Limits) Limits {
	out := limits
	if ceiling.MemoryMB > 0 && out.MemoryMB > ceiling.MemoryMB {
		out.MemoryMB = ceiling.MemoryMB
	}
	if ceiling.CPUQuota > 0 && out.CPUQuota > ceiling.CPUQuota {
		out.CPUQuota = ceiling.CPUQuota
	}
	if ceiling.PidsLimit > 0 && out.PidsLimit > ceiling.PidsLimit {
		out.PidsLimit = ceiling.PidsLimit
	}
	if ceiling.StorageGB > 0 && out.StorageGB > ceiling.StorageGB {
		out.StorageGB = ceiling.StorageGB
	}
	// NetworkMode is not capped
	return out
}
