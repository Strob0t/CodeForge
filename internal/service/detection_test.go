package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// --- detectFromGitRemote ---

func TestDetectFromGitRemote_GitHub(t *testing.T) {
	dets := detectFromGitRemote("https://github.com/owner/repo")
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	d := dets[0]
	if d.Provider != "github-issues" {
		t.Errorf("expected provider github-issues, got %q", d.Provider)
	}
	if d.ProjectRef != "owner/repo" {
		t.Errorf("expected project_ref owner/repo, got %q", d.ProjectRef)
	}
	if d.Source != "git_remote" {
		t.Errorf("expected source git_remote, got %q", d.Source)
	}
	if d.Confidence != "high" {
		t.Errorf("expected confidence high, got %q", d.Confidence)
	}
}

func TestDetectFromGitRemote_GitLab(t *testing.T) {
	dets := detectFromGitRemote("https://gitlab.com/org/project")
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	d := dets[0]
	if d.Provider != "gitlab" {
		t.Errorf("expected provider gitlab, got %q", d.Provider)
	}
	if d.ProjectRef != "org/project" {
		t.Errorf("expected project_ref org/project, got %q", d.ProjectRef)
	}
}

func TestDetectFromGitRemote_SSH(t *testing.T) {
	dets := detectFromGitRemote("git@github.com:owner/repo.git")
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	d := dets[0]
	if d.Provider != "github-issues" {
		t.Errorf("expected provider github-issues, got %q", d.Provider)
	}
	if d.ProjectRef != "owner/repo" {
		t.Errorf("expected project_ref owner/repo, got %q", d.ProjectRef)
	}
	if d.Source != "git_remote" {
		t.Errorf("expected source git_remote, got %q", d.Source)
	}
}

func TestDetectFromGitRemote_Empty(t *testing.T) {
	dets := detectFromGitRemote("")
	if dets != nil {
		t.Errorf("expected nil for empty URL, got %v", dets)
	}
}

func TestDetectFromGitRemote_UnknownHost(t *testing.T) {
	dets := detectFromGitRemote("https://custom.example.com/a/b")
	if dets != nil {
		t.Errorf("expected nil for unknown host, got %v", dets)
	}
}

func TestDetectFromGitRemote_Bitbucket(t *testing.T) {
	// Bitbucket is a known host in ParseRepoURL but has no PM provider mapping,
	// so it should return nil.
	dets := detectFromGitRemote("https://bitbucket.org/team/repo")
	if dets != nil {
		t.Errorf("expected nil for bitbucket (no PM provider), got %v", dets)
	}
}

func TestDetectFromGitRemote_GitLabSSH(t *testing.T) {
	dets := detectFromGitRemote("git@gitlab.com:org/project.git")
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	if dets[0].Provider != "gitlab" {
		t.Errorf("expected provider gitlab, got %q", dets[0].Provider)
	}
	if dets[0].ProjectRef != "org/project" {
		t.Errorf("expected project_ref org/project, got %q", dets[0].ProjectRef)
	}
}

// --- detectFromProjectConfig ---

func TestDetectFromProjectConfig_Plane(t *testing.T) {
	config := map[string]string{
		"plane_workspace":  "ws",
		"plane_project_id": "pid",
	}
	dets := detectFromProjectConfig(config)
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	d := dets[0]
	if d.Provider != "plane" {
		t.Errorf("expected provider plane, got %q", d.Provider)
	}
	if d.ProjectRef != "ws/pid" {
		t.Errorf("expected project_ref ws/pid, got %q", d.ProjectRef)
	}
	if d.Source != "config" {
		t.Errorf("expected source config, got %q", d.Source)
	}
	if d.Confidence != "high" {
		t.Errorf("expected confidence high, got %q", d.Confidence)
	}
}

func TestDetectFromProjectConfig_PlanePartial(t *testing.T) {
	// Only workspace, no project_id — should not detect.
	config := map[string]string{
		"plane_workspace": "ws",
	}
	dets := detectFromProjectConfig(config)
	// Filter out plane-specific detections.
	for _, d := range dets {
		if d.Provider == "plane" {
			t.Errorf("expected no plane detection with partial config, got %v", d)
		}
	}
}

func TestDetectFromProjectConfig_Empty(t *testing.T) {
	dets := detectFromProjectConfig(nil)
	if dets != nil {
		t.Errorf("expected nil for nil config, got %v", dets)
	}
}

func TestDetectFromProjectConfig_EmptyMap(t *testing.T) {
	dets := detectFromProjectConfig(map[string]string{})
	if dets != nil {
		t.Errorf("expected nil for empty config, got %v", dets)
	}
}

func TestDetectFromProjectConfig_Generic(t *testing.T) {
	config := map[string]string{
		"pm_provider":    "github-issues",
		"pm_project_ref": "o/r",
	}
	dets := detectFromProjectConfig(config)
	if len(dets) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(dets))
	}
	d := dets[0]
	if d.Provider != "github-issues" {
		t.Errorf("expected provider github-issues, got %q", d.Provider)
	}
	if d.ProjectRef != "o/r" {
		t.Errorf("expected project_ref o/r, got %q", d.ProjectRef)
	}
}

func TestDetectFromProjectConfig_GenericMissingRef(t *testing.T) {
	// pm_provider without pm_project_ref — should not detect.
	config := map[string]string{
		"pm_provider": "github-issues",
	}
	dets := detectFromProjectConfig(config)
	for _, d := range dets {
		if d.Provider == "github-issues" {
			t.Errorf("expected no generic detection without pm_project_ref, got %v", d)
		}
	}
}

func TestDetectFromProjectConfig_BothPlaneAndGeneric(t *testing.T) {
	config := map[string]string{
		"plane_workspace":  "ws",
		"plane_project_id": "pid",
		"pm_provider":      "github-issues",
		"pm_project_ref":   "o/r",
	}
	dets := detectFromProjectConfig(config)
	if len(dets) != 2 {
		t.Fatalf("expected 2 detections, got %d", len(dets))
	}
	providers := map[string]bool{}
	for _, d := range dets {
		providers[d.Provider] = true
	}
	if !providers["plane"] {
		t.Error("expected plane detection")
	}
	if !providers["github-issues"] {
		t.Error("expected github-issues detection")
	}
}

// --- detectPlatforms (integration of both sources + Available() filter) ---

// registerTestProviders registers test PM provider factories for the given names.
// It returns a cleanup function that should be deferred.
// NOTE: pmprovider.Register panics on duplicate registration, so we only register
// names that are not already registered.
func registerTestProviders(t *testing.T, names ...string) {
	t.Helper()
	available := pmprovider.Available()
	existingSet := map[string]bool{}
	for _, name := range available {
		existingSet[name] = true
	}
	for _, name := range names {
		if existingSet[name] {
			continue
		}
		pmprovider.Register(name, func(_ map[string]string) (pmprovider.Provider, error) {
			return nil, nil // factory never called in these tests
		})
	}
}

func TestDetectPlatforms_Multiple(t *testing.T) {
	registerTestProviders(t, "github-issues", "plane")

	proj := &project.Project{
		RepoURL: "https://github.com/owner/repo",
		Config: map[string]string{
			"plane_workspace":  "ws",
			"plane_project_id": "pid",
		},
	}
	platforms := detectPlatforms(proj)
	if len(platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d: %v", len(platforms), platforms)
	}
	providers := map[string]bool{}
	for _, p := range platforms {
		providers[p.Provider] = true
	}
	if !providers["github-issues"] {
		t.Error("expected github-issues detection")
	}
	if !providers["plane"] {
		t.Error("expected plane detection")
	}
}

func TestDetectPlatforms_Dedup(t *testing.T) {
	registerTestProviders(t, "github-issues")

	// Same provider from both git remote and generic config — should deduplicate.
	proj := &project.Project{
		RepoURL: "https://github.com/owner/repo",
		Config: map[string]string{
			"pm_provider":    "github-issues",
			"pm_project_ref": "other/ref",
		},
	}
	platforms := detectPlatforms(proj)
	count := 0
	for _, p := range platforms {
		if p.Provider == "github-issues" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 github-issues detection (dedup), got %d", count)
	}
}

func TestDetectPlatforms_FilteredByAvailable(t *testing.T) {
	// "nonexistent-provider" is not registered, so it should be filtered out.
	proj := &project.Project{
		Config: map[string]string{
			"pm_provider":    "nonexistent-provider",
			"pm_project_ref": "a/b",
		},
	}
	platforms := detectPlatforms(proj)
	for _, p := range platforms {
		if p.Provider == "nonexistent-provider" {
			t.Error("expected nonexistent-provider to be filtered out by Available()")
		}
	}
}

func TestDetectPlatforms_EmptyProject(t *testing.T) {
	proj := &project.Project{}
	platforms := detectPlatforms(proj)
	if len(platforms) != 0 {
		t.Errorf("expected 0 platforms for empty project, got %d", len(platforms))
	}
}

// --- PlatformDetection struct ---

func TestPlatformDetection_Fields(t *testing.T) {
	pd := roadmap.PlatformDetection{
		Provider:   "github-issues",
		ProjectRef: "owner/repo",
		Source:     "git_remote",
		Confidence: "high",
	}
	if pd.Provider != "github-issues" {
		t.Errorf("unexpected provider: %q", pd.Provider)
	}
	if pd.ProjectRef != "owner/repo" {
		t.Errorf("unexpected project_ref: %q", pd.ProjectRef)
	}
	if pd.Source != "git_remote" {
		t.Errorf("unexpected source: %q", pd.Source)
	}
	if pd.Confidence != "high" {
		t.Errorf("unexpected confidence: %q", pd.Confidence)
	}
}
