package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

// CommandAllowed tells whether the conversation's agent may run command
// with Bash on its own, as the tool-call evaluation of the conversation
// decides it: its effective policy profile (project > mode autonomy preset
// > default) allows the call, or asks under a profile that approves asks
// itself (accept-edits, delegate). reason says why not. The auto-agent runs
// its verification commands only then (KI-152 review). Without a policy
// service every command is allowed (no policy layer is configured).
func (s *ConversationService) CommandAllowed(ctx context.Context, conversationID, command string) (allowed bool, reason string, err error) {
	if s.policySvc == nil {
		return true, "", nil
	}
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return false, "", fmt.Errorf("get conversation: %w", err)
	}
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return false, "", fmt.Errorf("get project: %w", err)
	}
	modeID := conv.Mode
	if modeID == "" {
		modeID = defaultConversationMode
	}
	var m *mode.Mode
	autonomy := 0
	if s.modeSvc != nil {
		if found, modeErr := s.modeSvc.Get(modeID); modeErr == nil {
			m, autonomy = found, found.Autonomy
		}
	}
	profileName := effectivePolicyProfile(ctx, s.policySvc, conversationPolicyProfile(proj, autonomy, s.policySvc.DefaultProfile()), proj.ID)
	profile, ok := s.policySvc.GetProfile(ctx, profileName)
	if !ok {
		return false, fmt.Sprintf("the policy profile %q is unknown", profileName), nil
	}
	opts := []policy.EvalOption{policy.WithWorkspace(proj.WorkspacePath)}
	if m != nil {
		opts = append(opts, policy.WithModeTools(m.ID, m.Tools, m.DeniedTools))
	}
	decision, err := s.policySvc.Evaluate(ctx, profileName, policy.ToolCall{Tool: policy.ToolBash, Command: command}, opts...)
	if err != nil {
		return false, fmt.Sprintf("the policy profile %s could not decide: %v", profileName, err), nil
	}
	switch {
	case decision == policy.DecisionAllow:
		return true, "", nil
	case decision == policy.DecisionAsk && (profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate):
		return true, "", nil
	case decision == policy.DecisionAsk:
		return false, fmt.Sprintf("the policy profile %s asks before running it", profileName), nil
	default:
		return false, fmt.Sprintf("the policy profile %s does not allow running it", profileName), nil
	}
}
