package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	neturl "net/url"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// pmSyncer runs a roadmap sync (SyncService).
type pmSyncer interface {
	Sync(ctx context.Context, cfg *roadmap.SyncConfig) (*roadmap.SyncResult, error)
}

// PMWebhookService processes PM platform webhooks and triggers sync.
type PMWebhookService struct {
	hub   broadcast.Broadcaster
	sync  pmSyncer
	store database.Store
	// providerConfigs are the operator's credentials per registered PM
	// provider (plane: api_token, base_url; gitlab: token).
	providerConfigs map[string]map[string]string
}

// NewPMWebhookService creates a PM webhook service. providerConfigs holds
// the operator-configured settings per PM provider name.
func NewPMWebhookService(hub broadcast.Broadcaster, syncer pmSyncer, store database.Store, providerConfigs map[string]map[string]string) *PMWebhookService {
	return &PMWebhookService{hub: hub, sync: syncer, store: store, providerConfigs: providerConfigs}
}

// webhookProviders maps the webhook sources to the registered PM provider
// names (KI-56: GitHub issues are provided by "github-issues").
var webhookProviders = map[string]string{
	"github": "github-issues",
	"gitlab": "gitlab",
	"plane":  "plane",
}

// prepareSync finds the project a webhook refers to and builds the sync the
// provider can run: the registered provider name and its configuration (the
// operator's credentials, the project's settings). A webhook it cannot serve
// is an error - no project for the reference (domain.ErrNotFound), a
// provider that is not registered or not configured (domain.ErrValidation) -
// so the sender sees it instead of a silent success (KI-56).
func (s *PMWebhookService) prepareSync(ctx context.Context, source, projectRef string) (*roadmap.SyncConfig, error) {
	provider := webhookProviders[source]
	if provider == "" {
		return nil, fmt.Errorf("webhook source %q: %w", source, domain.ErrValidation)
	}
	proj, err := s.findProject(ctx, source, projectRef)
	if err != nil {
		return nil, err
	}
	if source == "plane" {
		// The provider addresses Plane projects by workspace slug; the
		// webhook names the workspace by its ID.
		projectRef = proj.Config["plane_workspace"] + "/" + proj.Config["plane_project_id"]
	}
	providerCfg, err := s.providerConfig(provider, proj)
	if err != nil {
		slog.Error("webhook: sync refused", "provider", provider, "project", proj.ID, "error", err)
		s.announce(ctx, &event.PMSyncEvent{ProjectID: proj.ID, Provider: provider, Status: "failed", Error: err.Error()})
		return nil, err
	}
	cfg := &roadmap.SyncConfig{
		ProjectID:      proj.ID,
		ProjectRef:     projectRef,
		Provider:       provider,
		Direction:      roadmap.SyncDirectionPull,
		CreateNew:      true,
		UpdateExist:    true,
		ProviderConfig: providerCfg,
	}
	// Constructing the provider checks its configuration (Plane requires
	// an api_token) before the webhook is accepted.
	if _, err := pmprovider.New(provider, cfg.ProviderConfig); err != nil {
		return nil, fmt.Errorf("%s webhook for project %s: %w: %w", source, proj.ID, err, domain.ErrValidation)
	}
	return cfg, nil
}

// findProject returns the project of the webhook's reference: the
// repository path (owner/name, GitLab with subgroups) for GitHub and
// GitLab, the project config key plane_project_id for Plane.
func (s *PMWebhookService) findProject(ctx context.Context, source, projectRef string) (*project.Project, error) {
	if source != "plane" {
		proj, err := s.store.GetProjectByRepoName(ctx, projectRef)
		if err != nil {
			return nil, fmt.Errorf("%s webhook: no project for repository %q: %w", source, projectRef, err)
		}
		// The store matches a substring of the repository URL; syncing
		// acme/app's issues into acme/app-private must not happen.
		if proj == nil {
			return nil, fmt.Errorf("%s webhook: no project for repository %q: %w", source, projectRef, domain.ErrNotFound)
		}
		if _, path, ok := repoURLParts(proj.RepoURL); !ok || !strings.EqualFold(path, projectRef) {
			return nil, fmt.Errorf("%s webhook: no project for repository %q (closest: %s): %w", source, projectRef, proj.RepoURL, domain.ErrNotFound)
		}
		return proj, nil
	}
	_, planeProject, _ := strings.Cut(projectRef, "/")
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("plane webhook: list projects: %w", err)
	}
	for i := range projects {
		c := projects[i].Config
		if planeProject != "" && c["plane_project_id"] == planeProject && c["plane_workspace"] != "" {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("plane webhook: no project for Plane project %q (config plane_workspace / plane_project_id): %w", planeProject, domain.ErrNotFound)
}

// repoURLParts splits a repository URL (https://host/path[.git] or
// git@host:path[.git]) into its base URL and repository path.
func repoURLParts(repoURL string) (base, path string, ok bool) {
	if rest, found := strings.CutPrefix(repoURL, "git@"); found {
		host, p, found := strings.Cut(rest, ":")
		if !found || host == "" {
			return "", "", false
		}
		base, path = "https://"+host, p
	} else {
		u, err := neturl.Parse(repoURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return "", "", false
		}
		base, path = u.Scheme+"://"+u.Host, u.Path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	return base, path, path != ""
}

// providerConfig is the operator's configuration of the provider plus what
// the project determines. The operator's credentials go only to the
// operator's host (S3-F security review S3): a project can name another
// host (plane_base_url, its repository URL), but never get the operator's
// token sent there.
//   - Plane: the base URL is the operator's (plane.base_url); a project
//     whose plane_base_url names another one is refused.
//   - GitLab: the base URL is the project's repository host. An operator
//     token is used only with the operator's base URL, and only for a
//     project on that host; without a token the sync is anonymous.
func (s *PMWebhookService) providerConfig(provider string, proj *project.Project) (map[string]string, error) {
	cfg := maps.Clone(s.providerConfigs[provider])
	if cfg == nil {
		cfg = map[string]string{}
	}
	switch provider {
	case "gitlab":
		base, _, ok := repoURLParts(proj.RepoURL)
		if !ok {
			return nil, fmt.Errorf("gitlab webhook: project %s has no GitLab repository URL: %w", proj.ID, domain.ErrValidation)
		}
		if cfg["token"] != "" && !sameBaseURL(cfg["base_url"], base) {
			return nil, fmt.Errorf("gitlab webhook: project %s is on %s, the operator's GitLab token is for %q: %w",
				proj.ID, base, cfg["base_url"], domain.ErrValidation)
		}
		cfg["base_url"] = base
	case "plane":
		if own := proj.Config["plane_base_url"]; own != "" && !sameBaseURL(own, cfg["base_url"]) {
			return nil, fmt.Errorf("plane webhook: project %s sets plane_base_url %q, not the operator's Plane %q "+
				"(plane.base_url) - the operator's token is not sent there: %w", proj.ID, own, cfg["base_url"], domain.ErrValidation)
		}
	}
	return cfg, nil
}

// sameBaseURL compares two base URLs, ignoring case of scheme and host and
// trailing slashes. An empty URL matches nothing.
func sameBaseURL(a, b string) bool {
	norm := func(raw string) string {
		u, err := neturl.Parse(strings.TrimRight(raw, "/"))
		if err != nil || u.Host == "" {
			return ""
		}
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath()
	}
	na := norm(a)
	return na != "" && na == norm(b)
}

// runSync runs a prepared sync in the background (it outlives the webhook
// request) and announces its outcome, a failure included, as a pm.sync
// event.
func (s *PMWebhookService) runSync(ctx context.Context, cfg *roadmap.SyncConfig) {
	result, err := s.sync.Sync(ctx, cfg)
	ev := event.PMSyncEvent{ProjectID: cfg.ProjectID, Provider: cfg.Provider, Status: "completed"}
	if err != nil {
		slog.Error("webhook: pull sync failed", "provider", cfg.Provider, "project", cfg.ProjectID, "error", err)
		ev.Status, ev.Error = "failed", err.Error()
	} else {
		slog.Info("webhook: pull sync completed", "provider", cfg.Provider, "project", cfg.ProjectID,
			"created", result.Created, "updated", result.Updated)
		ev.Created, ev.Updated = result.Created, result.Updated
	}
	s.announce(ctx, &ev)
}

// announce broadcasts the outcome of a webhook sync as a pm.sync event.
func (s *PMWebhookService) announce(ctx context.Context, ev *event.PMSyncEvent) {
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, "pm.sync", *ev)
	}
}

// accept prepares the sync of a webhook event and starts it.
func (s *PMWebhookService) accept(ctx context.Context, evt *webhook.PMWebhookEvent) (*webhook.PMWebhookEvent, error) {
	slog.Info(evt.Provider+" webhook received", "action", evt.Action, "item_id", evt.ItemID, "project_ref", evt.ProjectRef)
	cfg, err := s.prepareSync(ctx, evt.Provider, evt.ProjectRef)
	if err != nil {
		slog.Warn("webhook: sync not started", "provider", evt.Provider, "ref", evt.ProjectRef, "error", err)
		return nil, err
	}
	go s.runSync(detachTenant(ctx), cfg) //nolint:gosec // G118: sync must outlive webhook request; keeps its tenant
	return evt, nil
}

// HandleGitHubIssueWebhook processes a GitHub issue event webhook.
func (s *PMWebhookService) HandleGitHubIssueWebhook(ctx context.Context, data []byte) (*webhook.PMWebhookEvent, error) {
	var raw struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse github issue webhook: %w: %w", err, domain.ErrValidation)
	}
	return s.accept(ctx, &webhook.PMWebhookEvent{
		Provider:   "github",
		Action:     raw.Action,
		ItemID:     fmt.Sprintf("%d", raw.Issue.Number),
		ProjectRef: raw.Repository.FullName,
	})
}

// HandleGitLabIssueWebhook processes a GitLab issue event webhook.
func (s *PMWebhookService) HandleGitLabIssueWebhook(ctx context.Context, data []byte) (*webhook.PMWebhookEvent, error) {
	var raw struct {
		ObjectKind       string `json:"object_kind"`
		ObjectAttributes struct {
			IID    int    `json:"iid"`
			Action string `json:"action"`
			State  string `json:"state"`
		} `json:"object_attributes"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gitlab issue webhook: %w: %w", err, domain.ErrValidation)
	}
	return s.accept(ctx, &webhook.PMWebhookEvent{
		Provider:   "gitlab",
		Action:     raw.ObjectAttributes.Action,
		ItemID:     fmt.Sprintf("%d", raw.ObjectAttributes.IID),
		ProjectRef: raw.Project.PathWithNamespace,
	})
}

// HandlePlaneWebhook processes a Plane.so webhook event.
func (s *PMWebhookService) HandlePlaneWebhook(ctx context.Context, data []byte) (*webhook.PMWebhookEvent, error) {
	var raw struct {
		Event string `json:"event"`
		Data  struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			Workspace string `json:"workspace"`
			Project   string `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse plane webhook: %w: %w", err, domain.ErrValidation)
	}

	// Plane events are like "issue.created", "issue.updated"
	action := raw.Event
	if _, after, ok := strings.Cut(raw.Event, "."); ok {
		action = after
	}
	return s.accept(ctx, &webhook.PMWebhookEvent{
		Provider:   "plane",
		Action:     action,
		ItemID:     raw.Data.ID,
		ProjectRef: fmt.Sprintf("%s/%s", raw.Data.Workspace, raw.Data.Project),
	})
}
