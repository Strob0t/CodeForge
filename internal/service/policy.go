package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// ErrPolicyDirNotConfigured is returned by operations that must persist a
// profile (Allow-Always) when no policy directory is configured.
var ErrPolicyDirNotConfigured = errors.New("policy directory not configured (set policy.custom_dir or CODEFORGE_POLICY_DIR)")

// PolicyService evaluates tool calls against policy profiles and
// provides access to built-in presets and loaded custom policies.
//
// It is safe for concurrent use. Profiles are replaced as a whole and never
// modified in place, so a profile value read under the lock stays valid
// after the lock is released.
type PolicyService struct {
	defaultProfile string

	mu       sync.RWMutex
	profiles map[string]policy.PolicyProfile

	// persistMu serializes all profile changes together with their policy
	// directory writes, so the file on disk always holds the latest version.
	persistMu sync.Mutex
	dir       string
	// files maps each profile persisted in dir to the file that defines it.
	files map[string]string
}

// NewPolicyService creates a PolicyService with built-in presets
// and optional custom profiles. Custom profiles override presets
// with the same name (an operator decision made on disk).
func NewPolicyService(defaultProfile string, custom []policy.PolicyProfile) *PolicyService {
	profiles := make(map[string]policy.PolicyProfile)

	for _, name := range policy.PresetNames() {
		p, _ := policy.PresetByName(name)
		profiles[name] = p
	}

	for i := range custom {
		if policy.IsPreset(custom[i].Name) {
			slog.Warn("custom policy profile overrides built-in preset", "profile", custom[i].Name)
		}
		profiles[custom[i].Name] = custom[i]
	}

	return &PolicyService{
		defaultProfile: defaultProfile,
		profiles:       profiles,
	}
}

// LoadPolicyDir loads the custom profiles in dir (overriding presets of the
// same name, an operator decision made on disk) and enables persistence:
// profiles changed or deleted later are written to, or removed from, the
// file that defines them; new profiles are written to <name>.yaml. A
// missing dir is created on the first write. An empty dir keeps profiles in
// memory only.
func (s *PolicyService) LoadPolicyDir(dir string) error {
	var sources []policy.ProfileSource
	if dir != "" {
		loaded, err := policy.LoadSourcesFromDirectory(dir)
		if err != nil {
			return err
		}
		sources = loaded
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.dir = dir
	s.files = make(map[string]string, len(sources))
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range sources {
		p := sources[i].Profile
		if policy.IsPreset(p.Name) {
			slog.Warn("custom policy profile overrides built-in preset", "profile", p.Name, "file", sources[i].File)
		}
		s.profiles[p.Name] = p
		s.files[p.Name] = sources[i].File
	}
	return nil
}

// Evaluate checks a ToolCall against a named PolicyProfile and returns a Decision.
func (s *PolicyService) Evaluate(ctx context.Context, profileName string, call policy.ToolCall, opts ...policy.EvalOption) (policy.Decision, error) {
	result, err := s.EvaluateWithReason(ctx, profileName, call, opts...)
	if err != nil {
		return policy.DecisionDeny, err
	}
	return result.Decision, nil
}

// EvaluateWithReason checks a ToolCall against a named PolicyProfile and returns
// the full evaluation result including which rule matched and why. Tool names
// are canonicalized by the profile evaluation (policy.CanonicalTool).
func (s *PolicyService) EvaluateWithReason(_ context.Context, profileName string, call policy.ToolCall, opts ...policy.EvalOption) (*policy.EvaluationResult, error) {
	p, ok := s.GetProfile(profileName)
	if !ok {
		return nil, fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, profileName)
	}
	result := p.Evaluate(call, opts...)
	return &result, nil
}

// ResolveProfile determines the effective policy profile name using scope resolution:
// run-level override -> project-level -> service default.
func (s *PolicyService) ResolveProfile(runProfile, projectProfile string) string {
	if runProfile != "" {
		return runProfile
	}
	if projectProfile != "" {
		return projectProfile
	}
	return s.defaultProfile
}

// GetProfile returns a policy profile by name.
func (s *PolicyService) GetProfile(name string) (policy.PolicyProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[name]
	return p, ok
}

// ListProfiles returns all available profile names, sorted alphabetically.
func (s *PolicyService) ListProfiles() []string {
	s.mu.RLock()
	names := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	return names
}

// SaveProfile adds or replaces a custom policy profile and persists it when
// a policy directory is configured. Built-in presets cannot be replaced.
func (s *PolicyService) SaveProfile(profile *policy.PolicyProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("%w: %s", domain.ErrValidation, err.Error())
	}
	if policy.IsPreset(profile.Name) {
		return fmt.Errorf("policy profile %q is a built-in preset and cannot be overwritten: %w", profile.Name, domain.ErrConflict)
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	return s.storeLocked(profile)
}

// DeleteProfile removes a custom policy profile and its file. Built-in presets cannot be deleted.
func (s *PolicyService) DeleteProfile(name string) error {
	if policy.IsPreset(name) {
		return fmt.Errorf("cannot delete built-in preset %q", name)
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	if _, ok := s.GetProfile(name); !ok {
		return fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, name)
	}
	if file, ok := s.files[name]; ok {
		if err := os.Remove(filepath.Join(s.dir, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove policy file: %w", err)
		}
		delete(s.files, name)
	}
	s.mu.Lock()
	delete(s.profiles, name)
	s.mu.Unlock()
	return nil
}

// DefaultProfile returns the name of the default policy profile.
func (s *PolicyService) DefaultProfile() string {
	return s.defaultProfile
}

// PrependRule adds a PermissionRule to the front of a named profile's rule list.
// Returns an error if the profile is unknown or a built-in preset.
// If an identical rule already exists, it is a no-op (idempotent).
func (s *PolicyService) PrependRule(profileName string, rule *policy.PermissionRule) error {
	if policy.IsPreset(profileName) {
		return fmt.Errorf("cannot modify built-in preset %q", profileName)
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	_, err := s.prependRuleLocked(profileName, "", rule)
	return err
}

// prependRuleLocked prepends rule to the profile target. When target does
// not exist and cloneFrom is set, target is created as a copy of cloneFrom.
// The caller holds persistMu.
func (s *PolicyService) prependRuleLocked(target, cloneFrom string, rule *policy.PermissionRule) (*policy.PolicyProfile, error) {
	p, ok := s.GetProfile(target)
	if !ok {
		source, found := s.GetProfile(cloneFrom)
		if cloneFrom == "" || !found {
			return nil, fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, target)
		}
		p = source
		p.Name = target
		p.Description = fmt.Sprintf("Custom clone of %s", cloneFrom)
		p.Rules = append([]policy.PermissionRule(nil), source.Rules...)
	} else if p.HasRule(rule) {
		return &p, nil
	}
	p.Rules = append([]policy.PermissionRule{*rule}, p.Rules...)
	if err := s.storeLocked(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// storeLocked writes profile to the policy directory (if configured) and
// then publishes it in memory. The caller holds persistMu.
func (s *PolicyService) storeLocked(profile *policy.PolicyProfile) error {
	if s.dir != "" {
		if err := s.writeProfileFile(profile); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.profiles[profile.Name] = *profile
	s.mu.Unlock()
	return nil
}

// writeProfileFile writes the profile atomically (temp file + rename) so a
// crash never leaves a truncated YAML file that would block the next startup.
// The caller holds persistMu.
func (s *PolicyService) writeProfileFile(profile *policy.PolicyProfile) error {
	file, err := s.profileFile(profile.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("create policy dir: %w", err)
	}
	path := filepath.Join(s.dir, file)
	tmp := path + ".tmp"
	if err := policy.SaveToFile(tmp, profile); err != nil {
		return fmt.Errorf("persist policy file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("persist policy file: %w", err)
	}
	s.files[profile.Name] = file
	return nil
}

// profileFile returns the file in the policy directory that holds a
// profile: the file it was loaded from or last written to, else
// <name>.yaml for a new profile. A new profile's name must stay inside the
// policy directory, and its file must not exist yet: it could define
// another profile or be an operator's file that was not loaded. The caller
// holds persistMu.
func (s *PolicyService) profileFile(name string) (string, error) {
	if file, ok := s.files[name]; ok {
		return file, nil
	}
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%w: invalid policy profile name %q", domain.ErrValidation, name)
	}
	file := name + ".yaml"
	if _, err := os.Lstat(filepath.Join(s.dir, file)); err == nil {
		return "", fmt.Errorf("%w: policy file %s already exists and does not define profile %q", domain.ErrConflict, file, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check policy file: %w", err)
	}
	return file, nil
}

// projectPolicyResolver provides the project lookup needed by AllowAlways.
// AllowAlways never changes a project's profile selection.
type projectPolicyResolver interface {
	Get(ctx context.Context, id string) (*project.Project, error)
}

// AllowAlways adds a persistent "allow" rule for an approved tool call to the
// policy profile that decided it (profile, as named by the permission
// request; when empty, the project's explicit profile, else the service
// default). The rule goes into the project's clone of that profile
// ({profile}-custom-{projectID}), which replaces the profile for this
// project's calls only (effectivePolicyProfile); only a custom profile the
// project selects explicitly is extended in place. The project's profile
// selection never changes, so a call is never decided by a broader profile
// than before plus the approved rule. The rule uses the canonical tool name;
// for Bash it only allows commands whose every part runs one of the
// executables of the approved command.
func (s *PolicyService) AllowAlways(ctx context.Context, projects projectPolicyResolver, projectID, profile, tool, command string) (*policy.PolicyProfile, error) {
	rule, err := allowAlwaysRule(tool, command)
	if err != nil {
		return nil, err
	}

	proj, err := projects.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	explicit := projectPolicyProfile(proj)
	base := profile
	if base == "" {
		base = explicit
	}
	if base == "" {
		base = s.defaultProfile
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	if s.dir == "" {
		return nil, ErrPolicyDirNotConfigured
	}
	if _, ok := s.GetProfile(base); !ok {
		return nil, fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, base)
	}
	target := projectProfileClone(base, projectID)
	if _, cloned := s.GetProfile(target); !cloned && base == explicit && !policy.IsPreset(base) {
		target = base
	}
	updated, err := s.prependRuleLocked(target, base, &rule)
	if err != nil {
		return nil, fmt.Errorf("prepend rule: %w", err)
	}
	return updated, nil
}

// projectProfileClone names a project's Allow-Always clone of a profile.
func projectProfileClone(profile, projectID string) string {
	suffix := "-custom-" + projectID
	if strings.HasSuffix(profile, suffix) {
		return profile
	}
	return profile + suffix
}

// profileLookup is the read access to policy profiles that the profile
// resolution needs.
type profileLookup interface {
	GetProfile(name string) (policy.PolicyProfile, bool)
}

// effectivePolicyProfile returns the profile that decides a project's tool
// calls resolved to base: the project's Allow-Always clone of base when one
// exists, else base itself.
func effectivePolicyProfile(profiles profileLookup, base, projectID string) string {
	if base == "" || projectID == "" {
		return base
	}
	if clone := projectProfileClone(base, projectID); clone != base {
		if _, ok := profiles.GetProfile(clone); ok {
			return clone
		}
	}
	return base
}

// allowAlwaysRule builds the allow rule for an approved tool call.
func allowAlwaysRule(tool, command string) (policy.PermissionRule, error) {
	canonical := policy.CanonicalTool(tool)
	if canonical == "" || strings.ContainsAny(canonical, "*?[") {
		return policy.PermissionRule{}, fmt.Errorf("%w: tool %q cannot be allowed always", domain.ErrValidation, tool)
	}
	rule := policy.PermissionRule{
		Specifier: policy.ToolSpecifier{Tool: canonical},
		Decision:  policy.DecisionAllow,
	}
	if canonical == policy.ToolBash {
		exes, ok := policy.CommandExecutables(command)
		if !ok {
			return policy.PermissionRule{}, fmt.Errorf("%w: cannot derive an allow-always rule from command %q", domain.ErrValidation, command)
		}
		rule.CommandAllow = exes
	}
	return rule, nil
}

// projectPolicyProfile returns the policy profile a project selects
// explicitly: its policy_profile field, else config["policy_preset"].
func projectPolicyProfile(proj *project.Project) string {
	if proj.PolicyProfile != "" {
		return proj.PolicyProfile
	}
	return proj.Config["policy_preset"]
}
