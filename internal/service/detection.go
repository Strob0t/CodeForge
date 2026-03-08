package service

import (
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// hostToPMProvider maps git hosting domains to their PM provider names.
// These must match the names registered via pmprovider.Register().
var hostToPMProvider = map[string]string{
	"github.com": "github-issues",
	"gitlab.com": "gitlab",
}

// detectPlatforms detects PM platforms from project properties.
// It combines detections from git remote URL and project config,
// deduplicates by provider name, and filters to only return
// providers that are actually registered in pmprovider.
func detectPlatforms(proj *project.Project) []roadmap.PlatformDetection {
	var platforms []roadmap.PlatformDetection
	seen := map[string]bool{}

	// Source 1: Git remote URL.
	if dets := detectFromGitRemote(proj.RepoURL); dets != nil {
		for _, d := range dets {
			if !seen[d.Provider] {
				platforms = append(platforms, d)
				seen[d.Provider] = true
			}
		}
	}

	// Source 2: Project config.
	if dets := detectFromProjectConfig(proj.Config); dets != nil {
		for _, d := range dets {
			if !seen[d.Provider] {
				platforms = append(platforms, d)
				seen[d.Provider] = true
			}
		}
	}

	// Filter: only return platforms whose provider is actually registered.
	available := pmprovider.Available()
	availSet := make(map[string]bool, len(available))
	for _, name := range available {
		availSet[name] = true
	}

	filtered := platforms[:0]
	for _, p := range platforms {
		if availSet[p.Provider] {
			filtered = append(filtered, p)
		}
	}

	return filtered
}

// detectFromGitRemote extracts PM platform detections from a git remote URL.
// Returns nil if the URL is empty, unparseable, or maps to an unknown host.
func detectFromGitRemote(repoURL string) []roadmap.PlatformDetection {
	if repoURL == "" {
		return nil
	}

	parsed, err := project.ParseRepoURL(repoURL)
	if err != nil {
		return nil
	}

	pmProvider, ok := hostToPMProvider[parsed.Host]
	if !ok {
		return nil
	}

	return []roadmap.PlatformDetection{
		{
			Provider:   pmProvider,
			ProjectRef: parsed.Owner + "/" + parsed.Repo,
			Source:     "git_remote",
			Confidence: "high",
		},
	}
}

// detectFromProjectConfig extracts PM platform detections from a project's
// Config map. Supports:
//   - Plane: requires both "plane_workspace" and "plane_project_id"
//   - Generic: requires both "pm_provider" and "pm_project_ref"
//
// Returns nil if config is nil or contains no recognized PM keys.
func detectFromProjectConfig(config map[string]string) []roadmap.PlatformDetection {
	if len(config) == 0 {
		return nil
	}

	var dets []roadmap.PlatformDetection

	// Check for Plane-specific config keys.
	workspace := config["plane_workspace"]
	projectID := config["plane_project_id"]
	if workspace != "" && projectID != "" {
		dets = append(dets, roadmap.PlatformDetection{
			Provider:   "plane",
			ProjectRef: workspace + "/" + projectID,
			Source:     "config",
			Confidence: "high",
		})
	}

	// Check for generic PM config keys.
	pmProvider := config["pm_provider"]
	pmRef := config["pm_project_ref"]
	if pmProvider != "" && pmRef != "" {
		dets = append(dets, roadmap.PlatformDetection{
			Provider:   pmProvider,
			ProjectRef: pmRef,
			Source:     "config",
			Confidence: "high",
		})
	}

	if len(dets) == 0 {
		return nil
	}
	return dets
}
