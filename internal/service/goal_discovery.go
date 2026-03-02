package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

const maxGoalFileSize = 50 * 1024 // 50 KB

// GoalDiscoveryResult summarizes what was detected and imported.
type GoalDiscoveryResult struct {
	GoalsCreated int                `json:"goals_created"`
	Goals        []goal.ProjectGoal `json:"goals"`
}

// GoalDiscoveryService manages project goal discovery, CRUD, and context injection.
type GoalDiscoveryService struct {
	db database.Store
}

// NewGoalDiscoveryService creates a new GoalDiscoveryService.
func NewGoalDiscoveryService(db database.Store) *GoalDiscoveryService {
	return &GoalDiscoveryService{db: db}
}

// DetectAndImport scans a workspace for goal files, deletes previous auto-detected
// goals from each source, and creates new ProjectGoal records. Idempotent.
func (s *GoalDiscoveryService) DetectAndImport(ctx context.Context, projectID, workspacePath string) (*GoalDiscoveryResult, error) {
	detected := detectGoalFiles(workspacePath)
	if len(detected) == 0 {
		return &GoalDiscoveryResult{}, nil
	}

	// Group by source for idempotent delete-then-recreate.
	sources := make(map[string]bool)
	for i := range detected {
		sources[detected[i].Source] = true
	}
	for src := range sources {
		if err := s.db.DeleteProjectGoalsBySource(ctx, projectID, src); err != nil {
			return nil, fmt.Errorf("delete goals for source %q: %w", src, err)
		}
	}

	result := &GoalDiscoveryResult{}
	for i := range detected {
		detected[i].ProjectID = projectID
		detected[i].Enabled = true
		if err := s.db.CreateProjectGoal(ctx, &detected[i]); err != nil {
			return nil, fmt.Errorf("create goal %q: %w", detected[i].Title, err)
		}
		result.Goals = append(result.Goals, detected[i])
		result.GoalsCreated++
	}

	return result, nil
}

// Create creates a new project goal from a request.
func (s *GoalDiscoveryService) Create(ctx context.Context, projectID string, req *goal.CreateRequest) (*goal.ProjectGoal, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	g := &goal.ProjectGoal{
		ProjectID: projectID,
		Kind:      req.Kind,
		Title:     req.Title,
		Content:   req.Content,
		Source:    req.Source,
		Priority:  req.Priority,
		Enabled:   true,
	}
	if g.Source == "" {
		g.Source = "manual"
	}
	if g.Priority == 0 {
		g.Priority = 90
	}
	if err := s.db.CreateProjectGoal(ctx, g); err != nil {
		return nil, fmt.Errorf("create project goal: %w", err)
	}
	return g, nil
}

// Get retrieves a project goal by ID.
func (s *GoalDiscoveryService) Get(ctx context.Context, id string) (*goal.ProjectGoal, error) {
	return s.db.GetProjectGoal(ctx, id)
}

// List returns all goals for a project.
func (s *GoalDiscoveryService) List(ctx context.Context, projectID string) ([]goal.ProjectGoal, error) {
	return s.db.ListProjectGoals(ctx, projectID)
}

// ListEnabled returns enabled goals for a project, ordered by priority.
func (s *GoalDiscoveryService) ListEnabled(ctx context.Context, projectID string) ([]goal.ProjectGoal, error) {
	return s.db.ListEnabledGoals(ctx, projectID)
}

// Update updates a project goal.
func (s *GoalDiscoveryService) Update(ctx context.Context, id string, req goal.UpdateRequest) (*goal.ProjectGoal, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	g, err := s.db.GetProjectGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Kind != nil {
		g.Kind = *req.Kind
	}
	if req.Title != nil {
		g.Title = *req.Title
	}
	if req.Content != nil {
		g.Content = *req.Content
	}
	if req.Priority != nil {
		g.Priority = *req.Priority
	}
	if req.Enabled != nil {
		g.Enabled = *req.Enabled
	}
	if err := s.db.UpdateProjectGoal(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// Delete removes a project goal.
func (s *GoalDiscoveryService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteProjectGoal(ctx, id)
}

// AsContextEntries converts enabled goals into ContextEntry objects
// for injection into a ContextPack.
func (s *GoalDiscoveryService) AsContextEntries(ctx context.Context, projectID string) ([]cfcontext.ContextEntry, error) {
	goals, err := s.db.ListEnabledGoals(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return asContextEntries(goals), nil
}

// --- Detection logic (pure functions, no DB) ---

// detectGoalFiles scans a workspace directory for goal-relevant files
// and returns ProjectGoal stubs (no ID/ProjectID/TenantID yet).
func detectGoalFiles(workspacePath string) []goal.ProjectGoal {
	var goals []goal.ProjectGoal

	// Tier 1: GSD .planning/ directory
	goals = append(goals, detectGSD(workspacePath)...)

	// Tier 2: Agent instructions
	goals = append(goals, detectAgentInstructions(workspacePath)...)

	// Tier 3: Project docs
	goals = append(goals, detectProjectDocs(workspacePath)...)

	return goals
}

// detectGSD checks for GSD .planning/ files.
func detectGSD(root string) []goal.ProjectGoal {
	planDir := filepath.Join(root, ".planning")
	if _, err := os.Stat(planDir); err != nil {
		return nil
	}

	var goals []goal.ProjectGoal

	patterns := []struct {
		file     string
		kind     goal.GoalKind
		title    string
		priority int
	}{
		{"PROJECT.md", goal.KindVision, "Project Vision (GSD)", 95},
		{"REQUIREMENTS.md", goal.KindRequirement, "Requirements (GSD)", 90},
		{"STATE.md", goal.KindState, "Current State (GSD)", 80},
	}

	for _, p := range patterns {
		content := readGoalFile(filepath.Join(planDir, p.file))
		if content == "" {
			continue
		}
		goals = append(goals, goal.ProjectGoal{
			Kind:       p.kind,
			Title:      p.title,
			Content:    content,
			Source:     "gsd",
			SourcePath: ".planning/" + p.file,
			Priority:   p.priority,
		})
	}

	// Detect numbered context files: NN-CONTEXT.md
	contextRe := regexp.MustCompile(`^\d+-CONTEXT\.md$`)
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return goals
	}
	for _, e := range entries {
		if e.IsDir() || !contextRe.MatchString(e.Name()) {
			continue
		}
		content := readGoalFile(filepath.Join(planDir, e.Name()))
		if content == "" {
			continue
		}
		goals = append(goals, goal.ProjectGoal{
			Kind:       goal.KindContext,
			Title:      "Context: " + strings.TrimSuffix(e.Name(), ".md"),
			Content:    content,
			Source:     "gsd",
			SourcePath: ".planning/" + e.Name(),
			Priority:   75,
		})
	}

	return goals
}

// detectAgentInstructions checks for CLAUDE.md, .cursorrules, .clinerules.
func detectAgentInstructions(root string) []goal.ProjectGoal {
	var goals []goal.ProjectGoal

	patterns := []struct {
		file     string
		source   string
		title    string
		priority int
	}{
		{"CLAUDE.md", "claude_md", "Agent Instructions (CLAUDE.md)", 88},
		{".cursorrules", "cursorrules", "Agent Rules (.cursorrules)", 85},
		{".clinerules", "cursorrules", "Agent Rules (.clinerules)", 85},
	}

	for _, p := range patterns {
		content := readGoalFile(filepath.Join(root, p.file))
		if content == "" {
			continue
		}
		goals = append(goals, goal.ProjectGoal{
			Kind:       goal.KindConstraint,
			Title:      p.title,
			Content:    content,
			Source:     p.source,
			SourcePath: p.file,
			Priority:   p.priority,
		})
	}

	return goals
}

// detectProjectDocs checks for README.md, CONTRIBUTING.md, docs/architecture.md, docs/requirements.md.
func detectProjectDocs(root string) []goal.ProjectGoal {
	var goals []goal.ProjectGoal

	// README.md — first section only
	readmeContent := readGoalFile(filepath.Join(root, "README.md"))
	if readmeContent != "" {
		first := extractFirstSection(readmeContent)
		if first != "" {
			goals = append(goals, goal.ProjectGoal{
				Kind:       goal.KindVision,
				Title:      "Project Overview (README)",
				Content:    first,
				Source:     "readme",
				SourcePath: "README.md",
				Priority:   70,
			})
		}
	}

	// CONTRIBUTING.md
	contribContent := readGoalFile(filepath.Join(root, "CONTRIBUTING.md"))
	if contribContent != "" {
		goals = append(goals, goal.ProjectGoal{
			Kind:       goal.KindConstraint,
			Title:      "Contributing Guidelines",
			Content:    contribContent,
			Source:     "contributing",
			SourcePath: "CONTRIBUTING.md",
			Priority:   60,
		})
	}

	// docs/architecture.md or docs/ARCHITECTURE.md
	for _, name := range []string{"docs/architecture.md", "docs/ARCHITECTURE.md"} {
		content := readGoalFile(filepath.Join(root, name))
		if content != "" {
			goals = append(goals, goal.ProjectGoal{
				Kind:       goal.KindConstraint,
				Title:      "Architecture",
				Content:    content,
				Source:     "architecture",
				SourcePath: name,
				Priority:   75,
			})
			break
		}
	}

	// docs/requirements.md or docs/REQUIREMENTS.md
	for _, name := range []string{"docs/requirements.md", "docs/REQUIREMENTS.md"} {
		content := readGoalFile(filepath.Join(root, name))
		if content != "" {
			goals = append(goals, goal.ProjectGoal{
				Kind:       goal.KindRequirement,
				Title:      "Requirements",
				Content:    content,
				Source:     "requirements",
				SourcePath: name,
				Priority:   85,
			})
			break
		}
	}

	return goals
}

// readGoalFile reads a file, returning empty string if it doesn't exist, is binary, or exceeds maxGoalFileSize.
func readGoalFile(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxGoalFileSize {
		return ""
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from known directory + filename, not user input
	if err != nil {
		return ""
	}
	// Reject binary files (null bytes indicate non-text content).
	if bytes.ContainsRune(data, 0) {
		return ""
	}
	return string(data)
}

// extractFirstSection returns content up to (but not including) the second heading.
// A "heading" is any line starting with "# " or "## ". The first heading found is
// treated as the title; the section ends at the next heading of any level.
// Used for README.md to avoid importing the full file.
func extractFirstSection(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	foundHeading := false
	for _, line := range lines {
		isHeading := strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ")
		if isHeading && foundHeading {
			break
		}
		if isHeading {
			foundHeading = true
		}
		result = append(result, line)
	}
	out := strings.TrimSpace(strings.Join(result, "\n"))
	out = truncateUTF8(out, 2000)
	return out
}

// truncateUTF8 truncates s to at most maxBytes without breaking a multi-byte rune.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from maxBytes to find a valid rune boundary.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// --- Context rendering ---

// asContextEntries converts goals to ContextEntry objects.
func asContextEntries(goals []goal.ProjectGoal) []cfcontext.ContextEntry {
	if len(goals) == 0 {
		return nil
	}
	entries := make([]cfcontext.ContextEntry, 0, len(goals))
	for i := range goals {
		entries = append(entries, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryGoal,
			Path:     goals[i].SourcePath,
			Content:  goals[i].Content,
			Tokens:   cfcontext.EstimateTokens(goals[i].Content),
			Priority: goals[i].Priority,
		})
	}
	return entries
}

// renderGoalContext assembles goals grouped by kind into structured markdown
// for injection into the system prompt.
func renderGoalContext(goals []goal.ProjectGoal) string {
	if len(goals) == 0 {
		return ""
	}

	grouped := make(map[goal.GoalKind][]goal.ProjectGoal)
	for i := range goals {
		grouped[goals[i].Kind] = append(grouped[goals[i].Kind], goals[i])
	}

	var sb strings.Builder
	sb.WriteString("## Project Goals\n")

	sections := []struct {
		kind  goal.GoalKind
		title string
	}{
		{goal.KindVision, "Vision"},
		{goal.KindRequirement, "Requirements"},
		{goal.KindConstraint, "Constraints & Decisions"},
		{goal.KindState, "Current State"},
		{goal.KindContext, "Context"},
	}

	for _, sec := range sections {
		items, ok := grouped[sec.kind]
		if !ok {
			continue
		}
		sb.WriteString("\n### " + sec.title + "\n\n")
		for i := range items {
			if len(items) > 1 {
				sb.WriteString("**" + items[i].Title + "**\n\n")
			}
			sb.WriteString(items[i].Content)
			if !strings.HasSuffix(items[i].Content, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
