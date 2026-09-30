package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// PolicyService evaluates tool calls against policy profiles and
// provides access to built-in presets and loaded custom policies.
type PolicyService struct {
	defaultProfile string
	profiles       map[string]policy.PolicyProfile
}

// NewPolicyService creates a PolicyService with built-in presets
// and optional custom profiles. Custom profiles override presets
// with the same name.
func NewPolicyService(defaultProfile string, custom []policy.PolicyProfile) *PolicyService {
	profiles := make(map[string]policy.PolicyProfile)

	// Register built-in presets.
	for _, name := range policy.PresetNames() {
		p, _ := policy.PresetByName(name)
		profiles[name] = p
	}

	// Register custom profiles (override presets if same name).
	for i := range custom {
		profiles[custom[i].Name] = custom[i]
	}

	return &PolicyService{
		defaultProfile: defaultProfile,
		profiles:       profiles,
	}
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
	p, ok := s.profiles[profileName]
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
	p, ok := s.profiles[name]
	return p, ok
}

// ListProfiles returns all available profile names, sorted alphabetically.
func (s *PolicyService) ListProfiles() []string {
	names := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SaveProfile adds or replaces a policy profile in the service.
func (s *PolicyService) SaveProfile(profile *policy.PolicyProfile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	s.profiles[profile.Name] = *profile
	return nil
}

// DeleteProfile removes a custom policy profile. Built-in presets cannot be deleted.
func (s *PolicyService) DeleteProfile(name string) error {
	if policy.IsPreset(name) {
		return fmt.Errorf("cannot delete built-in preset %q", name)
	}
	if _, ok := s.profiles[name]; !ok {
		return fmt.Errorf("unknown policy profile %q", name)
	}
	delete(s.profiles, name)
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
	p, ok := s.profiles[profileName]
	if !ok {
		return fmt.Errorf("%w: unknown policy profile %q", domain.ErrNotFound, profileName)
	}
	if p.HasRule(rule) {
		return nil // idempotent
	}
	p.Rules = append([]policy.PermissionRule{*rule}, p.Rules...)
	s.profiles[profileName] = p
	return nil
}

// projectPolicyResolver provides the project-level operations needed by AllowAlways.
type projectPolicyResolver interface {
	Get(ctx context.Context, id string) (*project.Project, error)
	SetPolicyProfile(ctx context.Context, projectID, profile string) error
}

// AllowAlways adds a persistent "allow" rule for a tool to a project's policy
// profile. If the project uses a built-in preset, a custom clone is created.
// This encapsulates the business logic previously in the HTTP handler.
func (s *PolicyService) AllowAlways(ctx context.Context, projects projectPolicyResolver, policyDir, projectID, tool, command string) (*policy.PolicyProfile, error) {
	proj, err := projects.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	effectiveProfile := s.ResolveProfile("", proj.PolicyProfile)

	if policy.IsPreset(effectiveProfile) {
		source, _ := s.GetProfile(effectiveProfile)
		cloneName := effectiveProfile + "-custom-" + projectID
		clone := source
		clone.Name = cloneName
		clone.Description = fmt.Sprintf("Custom clone of %s for project %s", effectiveProfile, projectID)

		if _, exists := s.GetProfile(cloneName); !exists {
			if err := s.SaveProfile(&clone); err != nil {
				return nil, fmt.Errorf("save cloned profile: %w", err)
			}
		}

		if err := projects.SetPolicyProfile(ctx, projectID, cloneName); err != nil {
			return nil, fmt.Errorf("set project policy profile: %w", err)
		}
		effectiveProfile = cloneName
	}

	spec := policy.ToolSpecifier{Tool: tool}
	if command != "" {
		parts := strings.SplitN(command, " ", 2)
		spec.SubPattern = parts[0] + "*"
	}
	rule := policy.PermissionRule{
		Specifier: spec,
		Decision:  policy.DecisionAllow,
	}

	if err := s.PrependRule(effectiveProfile, &rule); err != nil {
		return nil, fmt.Errorf("prepend rule: %w", err)
	}

	if policyDir != "" {
		updated, ok := s.GetProfile(effectiveProfile)
		if ok {
			path := filepath.Join(policyDir, effectiveProfile+".yaml")
			if mkErr := os.MkdirAll(policyDir, 0o750); mkErr != nil {
				return nil, fmt.Errorf("create policy dir: %w", mkErr)
			}
			if saveErr := policy.SaveToFile(path, &updated); saveErr != nil {
				return nil, fmt.Errorf("persist policy file: %w", saveErr)
			}
		}
	}

	result, _ := s.GetProfile(effectiveProfile)
	return &result, nil
}
