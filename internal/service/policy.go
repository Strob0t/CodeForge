package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ErrPolicyDirNotConfigured is returned by operations that must persist a
// profile (Allow-Always) when no policy directory is configured.
var ErrPolicyDirNotConfigured = errors.New("policy directory not configured (set policy.custom_dir or CODEFORGE_POLICY_DIR)")

// tenantDirPattern is the form of a tenant ID that may name a directory in
// the policy directory (a UUID, as the tenant middleware accepts).
var tenantDirPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// PolicyService evaluates tool calls against policy profiles and
// provides access to built-in presets and custom profiles.
//
// Custom profiles belong to a tenant (KI-68): the tenant in the context of
// every call (tenantctx; the default tenant when none is set) selects them.
// A profile name resolves to the tenant's own profile, then (default tenant
// only) a legacy file directly in the policy directory, then the built-in
// preset of that name. Presets are shared by all tenants and cannot be
// changed. Custom profiles are stored in <policy dir>/<tenant id>/; files
// directly in the policy directory (the layout before tenants) are loaded
// for the default tenant and never written or removed: a change is saved as
// the default tenant's own copy, which takes precedence.
//
// It is safe for concurrent use. Profiles are replaced as a whole and never
// modified in place, so a profile value read under the lock stays valid
// after the lock is released.
type PolicyService struct {
	defaultProfile string

	mu      sync.RWMutex
	presets map[string]policy.PolicyProfile
	// legacy holds the profiles of the files directly in the policy
	// directory: read-only, default tenant only.
	legacy map[string]policy.PolicyProfile
	// tenants holds each tenant's own profiles.
	tenants map[string]map[string]policy.PolicyProfile

	// persistMu serializes all profile changes together with their policy
	// directory writes, so the file on disk always holds the latest version.
	persistMu sync.Mutex
	dir       string
	// files maps each tenant's profiles persisted in <dir>/<tenant> to the
	// file that defines them.
	files map[string]map[string]string
}

// NewPolicyService creates a PolicyService with built-in presets and
// optional custom profiles of the default tenant. A custom profile may
// override a preset of the same name for that tenant (an operator decision).
func NewPolicyService(defaultProfile string, custom []policy.PolicyProfile) *PolicyService {
	presets := make(map[string]policy.PolicyProfile)
	for _, name := range policy.PresetNames() {
		p, _ := policy.PresetByName(name)
		presets[name] = p
	}

	own := make(map[string]policy.PolicyProfile, len(custom))
	for i := range custom {
		if policy.IsPreset(custom[i].Name) {
			slog.Warn("custom policy profile overrides built-in preset", "profile", custom[i].Name)
		}
		own[custom[i].Name] = custom[i]
	}

	return &PolicyService{
		defaultProfile: defaultProfile,
		presets:        presets,
		legacy:         make(map[string]policy.PolicyProfile),
		tenants:        map[string]map[string]policy.PolicyProfile{tenantctx.DefaultTenantID: own},
		files:          make(map[string]map[string]string),
	}
}

// LoadPolicyDir loads the custom profiles in dir and enables persistence.
// Each subdirectory named by a tenant ID holds that tenant's profiles; files
// directly in dir are loaded read-only for the default tenant (see
// PolicyService). Profiles changed or deleted later are written to, or
// removed from, the tenant file that defines them; new profiles are written
// to <dir>/<tenant>/<name>.yaml. A missing dir is created on the first
// write. An empty dir keeps profiles in memory only.
func (s *PolicyService) LoadPolicyDir(dir string) error {
	var legacy []policy.ProfileSource
	perTenant := make(map[string][]policy.ProfileSource)
	if dir != "" {
		loaded, err := policy.LoadSourcesFromDirectory(dir)
		if err != nil {
			return err
		}
		legacy = loaded
		if perTenant, err = loadTenantDirs(dir); err != nil {
			return err
		}
	}

	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.dir = dir
	s.files = make(map[string]map[string]string, len(perTenant))
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range legacy {
		p := legacy[i].Profile
		if policy.IsPreset(p.Name) {
			slog.Warn("custom policy profile overrides built-in preset", "profile", p.Name, "file", legacy[i].File)
		}
		s.legacy[p.Name] = p
	}
	for tenant, sources := range perTenant {
		own := s.ownProfilesLocked(tenant)
		files := make(map[string]string, len(sources))
		for i := range sources {
			p := sources[i].Profile
			if policy.IsPreset(p.Name) {
				slog.Warn("custom policy profile overrides built-in preset", "profile", p.Name, "tenant", tenant, "file", sources[i].File)
			}
			own[p.Name] = p
			files[p.Name] = sources[i].File
		}
		s.files[tenant] = files
	}
	return nil
}

// loadTenantDirs loads the profiles of every tenant subdirectory of dir.
func loadTenantDirs(dir string) (map[string][]policy.ProfileSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read policy directory %s: %w", dir, err)
	}
	perTenant := make(map[string][]policy.ProfileSource)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !tenantDirPattern.MatchString(entry.Name()) {
			slog.Warn("policy directory entry is not a tenant ID, ignored", "dir", entry.Name())
			continue
		}
		sources, err := policy.LoadSourcesFromDirectory(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		perTenant[entry.Name()] = sources
	}
	return perTenant, nil
}

// ownProfilesLocked returns the tenant's profile map, creating it. The
// caller holds mu for writing.
func (s *PolicyService) ownProfilesLocked(tenant string) map[string]policy.PolicyProfile {
	own, ok := s.tenants[tenant]
	if !ok {
		own = make(map[string]policy.PolicyProfile)
		s.tenants[tenant] = own
	}
	return own
}

// policyTenant returns the tenant whose profiles a call in ctx uses.
func policyTenant(ctx context.Context) string {
	return tenantctx.FromContext(ctx)
}

// lookup resolves a profile name in a tenant: its own profile, then a
// legacy file (default tenant), then the preset.
func (s *PolicyService) lookup(tenant, name string) (policy.PolicyProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.tenants[tenant][name]; ok {
		return p, true
	}
	if tenant == tenantctx.DefaultTenantID {
		if p, ok := s.legacy[name]; ok {
			return p, true
		}
	}
	p, ok := s.presets[name]
	return p, ok
}

// Evaluate checks a ToolCall against a named PolicyProfile of ctx's tenant and returns a Decision.
func (s *PolicyService) Evaluate(ctx context.Context, profileName string, call policy.ToolCall, opts ...policy.EvalOption) (policy.Decision, error) {
	result, err := s.EvaluateWithReason(ctx, profileName, call, opts...)
	if err != nil {
		return policy.DecisionDeny, err
	}
	return result.Decision, nil
}

// EvaluateWithReason checks a ToolCall against a named PolicyProfile of
// ctx's tenant and returns the full evaluation result including which rule
// matched and why. Tool names are canonicalized by the profile evaluation
// (policy.CanonicalTool).
func (s *PolicyService) EvaluateWithReason(ctx context.Context, profileName string, call policy.ToolCall, opts ...policy.EvalOption) (*policy.EvaluationResult, error) {
	p, ok := s.GetProfile(ctx, profileName)
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

// GetProfile returns a policy profile of ctx's tenant by name.
func (s *PolicyService) GetProfile(ctx context.Context, name string) (policy.PolicyProfile, bool) {
	return s.lookup(policyTenant(ctx), name)
}

// ListProfiles returns the profile names available to ctx's tenant (the
// presets and its own profiles), sorted alphabetically.
func (s *PolicyService) ListProfiles(ctx context.Context) []string {
	tenant := policyTenant(ctx)
	s.mu.RLock()
	seen := make(map[string]bool, len(s.presets)+len(s.tenants[tenant]))
	for name := range s.presets {
		seen[name] = true
	}
	if tenant == tenantctx.DefaultTenantID {
		for name := range s.legacy {
			seen[name] = true
		}
	}
	for name := range s.tenants[tenant] {
		seen[name] = true
	}
	s.mu.RUnlock()
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SaveProfile adds or replaces a custom policy profile of ctx's tenant and
// persists it when a policy directory is configured. Built-in presets
// cannot be replaced.
func (s *PolicyService) SaveProfile(ctx context.Context, profile *policy.PolicyProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("%w: %s", domain.ErrValidation, err.Error())
	}
	if policy.IsPreset(profile.Name) {
		return fmt.Errorf("policy profile %q is a built-in preset and cannot be overwritten: %w", profile.Name, domain.ErrConflict)
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	return s.storeLocked(policyTenant(ctx), profile)
}

// DeleteProfile removes a custom policy profile of ctx's tenant and its
// file. Built-in presets and legacy files cannot be deleted.
func (s *PolicyService) DeleteProfile(ctx context.Context, name string) error {
	if policy.IsPreset(name) {
		return fmt.Errorf("cannot delete built-in preset %q", name)
	}
	tenant := policyTenant(ctx)
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.mu.RLock()
	_, own := s.tenants[tenant][name]
	_, legacy := s.legacy[name]
	s.mu.RUnlock()
	if !own {
		if legacy && tenant == tenantctx.DefaultTenantID {
			return fmt.Errorf("%w: policy profile %q is defined by a file in the policy directory; remove it there", domain.ErrConflict, name)
		}
		return fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, name)
	}
	if file, ok := s.files[tenant][name]; ok {
		if err := os.Remove(filepath.Join(s.dir, tenant, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove policy file: %w", err)
		}
		delete(s.files[tenant], name)
	}
	s.mu.Lock()
	delete(s.tenants[tenant], name)
	s.mu.Unlock()
	return nil
}

// DefaultProfile returns the name of the default policy profile.
func (s *PolicyService) DefaultProfile() string {
	return s.defaultProfile
}

// PrependRule adds a PermissionRule to the front of a named profile's rule
// list in ctx's tenant. Returns an error if the profile is unknown or a
// built-in preset. If an identical rule already exists, it is a no-op
// (idempotent).
func (s *PolicyService) PrependRule(ctx context.Context, profileName string, rule *policy.PermissionRule) error {
	if policy.IsPreset(profileName) {
		return fmt.Errorf("cannot modify built-in preset %q", profileName)
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	_, err := s.prependRuleLocked(policyTenant(ctx), profileName, "", rule)
	return err
}

// prependRuleLocked prepends rule to the tenant's profile target. When
// target does not exist and cloneFrom is set, target is created as a copy
// of cloneFrom. The caller holds persistMu.
func (s *PolicyService) prependRuleLocked(tenant, target, cloneFrom string, rule *policy.PermissionRule) (*policy.PolicyProfile, error) {
	p, ok := s.lookup(tenant, target)
	if !ok {
		source, found := s.lookup(tenant, cloneFrom)
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
	if err := s.storeLocked(tenant, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// storeLocked writes profile to the tenant's policy directory (if
// configured) and then publishes it in memory. The caller holds persistMu.
func (s *PolicyService) storeLocked(tenant string, profile *policy.PolicyProfile) error {
	if s.dir != "" {
		if err := s.writeProfileFile(tenant, profile); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.ownProfilesLocked(tenant)[profile.Name] = *profile
	s.mu.Unlock()
	return nil
}

// writeProfileFile writes the profile to <dir>/<tenant> atomically (temp
// file + rename) so a crash never leaves a truncated YAML file that would
// block the next startup. The caller holds persistMu.
func (s *PolicyService) writeProfileFile(tenant string, profile *policy.PolicyProfile) error {
	if !tenantDirPattern.MatchString(tenant) {
		return fmt.Errorf("%w: invalid tenant ID %q for the policy directory", domain.ErrValidation, tenant)
	}
	file, err := s.profileFile(tenant, profile.Name)
	if err != nil {
		return err
	}
	tenantDir := filepath.Join(s.dir, tenant)
	if err := os.MkdirAll(tenantDir, 0o750); err != nil {
		return fmt.Errorf("create policy dir: %w", err)
	}
	path := filepath.Join(tenantDir, file)
	tmp := path + ".tmp"
	if err := policy.SaveToFile(tmp, profile); err != nil {
		return fmt.Errorf("persist policy file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("persist policy file: %w", err)
	}
	if s.files[tenant] == nil {
		s.files[tenant] = make(map[string]string)
	}
	s.files[tenant][profile.Name] = file
	return nil
}

// profileFile returns the file in the tenant's policy directory that holds
// a profile: the file it was loaded from or last written to, else
// <name>.yaml for a new profile. A new profile's name must stay inside the
// directory, and its file must not exist yet: it could define another
// profile or be an operator's file that was not loaded. The caller holds
// persistMu.
func (s *PolicyService) profileFile(tenant, name string) (string, error) {
	if file, ok := s.files[tenant][name]; ok {
		return file, nil
	}
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%w: invalid policy profile name %q", domain.ErrValidation, name)
	}
	file := name + ".yaml"
	if _, err := os.Lstat(filepath.Join(s.dir, tenant, file)); err == nil {
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
// executables of the approved command. The clone belongs to the tenant that
// owns the project.
func (s *PolicyService) AllowAlways(ctx context.Context, projects projectPolicyResolver, projectID, profile, tool, command string) (*policy.PolicyProfile, error) {
	rule, err := allowAlwaysRule(tool, command)
	if err != nil {
		return nil, err
	}

	proj, err := projects.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	tenant := proj.TenantID
	if tenant == "" {
		tenant = policyTenant(ctx)
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
	if _, ok := s.lookup(tenant, base); !ok {
		return nil, fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, base)
	}
	target := projectProfileClone(base, projectID)
	if _, cloned := s.lookup(tenant, target); !cloned && base == explicit && !policy.IsPreset(base) {
		target = base
	}
	updated, err := s.prependRuleLocked(tenant, target, base, &rule)
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
	GetProfile(ctx context.Context, name string) (policy.PolicyProfile, bool)
}

// effectivePolicyProfile returns the profile that decides a project's tool
// calls resolved to base: the project's Allow-Always clone of base in ctx's
// tenant when one exists, else base itself.
func effectivePolicyProfile(ctx context.Context, profiles profileLookup, base, projectID string) string {
	if base == "" || projectID == "" {
		return base
	}
	if clone := projectProfileClone(base, projectID); clone != base {
		if _, ok := profiles.GetProfile(ctx, clone); ok {
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
			return policy.PermissionRule{}, fmt.Errorf("%w: cannot derive an allow-always rule from command %q "+
				"(it cannot be analysed statically, or it sets PYTHONPATH or NODE_PATH, which no allow rule matches)", domain.ErrValidation, command)
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
