package service

import (
	"context"
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
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// pmSyncer runs a roadmap sync (SyncService).
type pmSyncer interface {
	Sync(ctx context.Context, cfg *roadmap.SyncConfig) (*roadmap.SyncResult, error)
}

// PMWebhookService turns a PM webhook event into a pull sync of the
// webhook's project (KI-56, KI-85).
type PMWebhookService struct {
	hub  broadcast.Broadcaster
	sync pmSyncer
	// providerConfigs are the operator's credentials per registered PM
	// provider (plane: api_token, base_url). They serve the default
	// tenant only.
	providerConfigs map[string]map[string]string
}

// NewPMWebhookService creates a PM webhook service. providerConfigs holds
// the operator-configured settings per PM provider name.
func NewPMWebhookService(hub broadcast.Broadcaster, syncer pmSyncer, providerConfigs map[string]map[string]string) *PMWebhookService {
	return &PMWebhookService{hub: hub, sync: syncer, providerConfigs: providerConfigs}
}

// webhookProviders maps the webhook sources to the registered PM provider
// names (KI-56: GitHub issues are provided by "github-issues").
var webhookProviders = map[string]string{
	"github": "github-issues",
	"gitlab": "gitlab",
	"plane":  "plane",
}

// pmEvent is a parsed PM webhook event with what identifies its repository
// or Plane project.
type pmEvent struct {
	webhook.PMWebhookEvent
	repo         webhookRepository // GitHub, GitLab
	planeProject string            // Plane project ID
}

// HandleEvent starts the pull sync of proj for a PM webhook event of source
// (github, gitlab, plane). apiToken is the integration's own token for the
// provider's API ("" for none). The event must be about proj's repository
// (GitHub, GitLab) or Plane project (webhook.ErrRepositoryMismatch
// otherwise, KI-85); a payload that cannot be read or a provider that
// cannot sync is a domain.ErrValidation, so the sender sees it instead of a
// silent success (KI-56). The sync runs in the background in ctx's tenant
// and announces its outcome as a pm.sync event.
func (s *PMWebhookService) HandleEvent(ctx context.Context, source string, proj *project.Project, apiToken string, data []byte) (*webhook.PMWebhookEvent, error) {
	provider := webhookProviders[source]
	if provider == "" {
		return nil, fmt.Errorf("webhook source %q: %w", source, domain.ErrValidation)
	}
	ev, err := parsePMEvent(source, data)
	if err != nil {
		return nil, err
	}
	slog.Info(source+" webhook received", "project_id", proj.ID, "action", ev.Action, "item_id", ev.ItemID, "project_ref", ev.ProjectRef)

	projectRef := ev.ProjectRef
	if source == "plane" {
		if ev.planeProject == "" || ev.planeProject != proj.Config["plane_project_id"] {
			slog.Warn("webhook event for another Plane project ignored", "project_id", proj.ID, "event_project", ev.planeProject)
			return nil, fmt.Errorf("plane event for project %q, project %s is %q (config plane_project_id): %w",
				ev.planeProject, proj.ID, proj.Config["plane_project_id"], webhook.ErrRepositoryMismatch)
		}
		// The provider addresses Plane projects by workspace slug; the
		// webhook names the workspace by its ID.
		projectRef = proj.Config["plane_workspace"] + "/" + proj.Config["plane_project_id"]
	} else if err := checkRepository(proj, source, ev.repo); err != nil {
		return nil, err
	}

	providerCfg, err := s.providerConfig(ctx, provider, proj, apiToken)
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
	go s.runSync(detachTenant(ctx), cfg) //nolint:gosec // G118: sync must outlive webhook request; keeps its tenant
	return &ev.PMWebhookEvent, nil
}

// parsePMEvent reads the event of a PM webhook payload.
func parsePMEvent(source string, data []byte) (*pmEvent, error) {
	switch source {
	case "github":
		var raw struct {
			Action string `json:"action"`
			Issue  struct {
				Number int `json:"number"`
			} `json:"issue"`
			Repository struct {
				FullName string `json:"full_name"`
				HTMLURL  string `json:"html_url"` // GitHub Enterprise: its own host
			} `json:"repository"`
		}
		if err := parsePayload(data, "github issue webhook", &raw); err != nil {
			return nil, err
		}
		return &pmEvent{
			PMWebhookEvent: webhook.PMWebhookEvent{Provider: source, Action: raw.Action, ItemID: fmt.Sprintf("%d", raw.Issue.Number), ProjectRef: raw.Repository.FullName},
			repo:           webhookRepository{raw.Repository.FullName, raw.Repository.HTMLURL},
		}, nil
	case "gitlab":
		var raw struct {
			ObjectAttributes struct {
				IID    int    `json:"iid"`
				Action string `json:"action"`
			} `json:"object_attributes"`
			Project struct {
				PathWithNamespace string `json:"path_with_namespace"`
				WebURL            string `json:"web_url"` // self-hosted GitLab: its own host
			} `json:"project"`
		}
		if err := parsePayload(data, "gitlab issue webhook", &raw); err != nil {
			return nil, err
		}
		return &pmEvent{
			PMWebhookEvent: webhook.PMWebhookEvent{Provider: source, Action: raw.ObjectAttributes.Action, ItemID: fmt.Sprintf("%d", raw.ObjectAttributes.IID), ProjectRef: raw.Project.PathWithNamespace},
			repo:           webhookRepository{raw.Project.PathWithNamespace, raw.Project.WebURL},
		}, nil
	default: // plane
		var raw struct {
			Event string `json:"event"`
			Data  struct {
				ID        string `json:"id"`
				Workspace string `json:"workspace"`
				Project   string `json:"project"`
			} `json:"data"`
		}
		if err := parsePayload(data, "plane webhook", &raw); err != nil {
			return nil, err
		}
		// Plane events are like "issue.created", "issue.updated".
		action := raw.Event
		if _, after, ok := strings.Cut(raw.Event, "."); ok {
			action = after
		}
		return &pmEvent{
			PMWebhookEvent: webhook.PMWebhookEvent{Provider: source, Action: action, ItemID: raw.Data.ID, ProjectRef: raw.Data.Workspace + "/" + raw.Data.Project},
			planeProject:   raw.Data.Project,
		}, nil
	}
}

// repoBaseURL returns the base URL (scheme://host[:port]) of a repository
// URL (https://host/path[.git] or git@host:path[.git]); ok is false when it
// names no repository path.
func repoBaseURL(repoURL string) (base string, ok bool) {
	var path string
	if rest, found := strings.CutPrefix(repoURL, "git@"); found {
		host, p, found := strings.Cut(rest, ":")
		if !found || host == "" {
			return "", false
		}
		base, path = "https://"+host, p
	} else {
		u, err := neturl.Parse(repoURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return "", false
		}
		base, path = u.Scheme+"://"+u.Host, u.Path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	return base, path != ""
}

// providerConfig is what the sync of proj authenticates with:
//   - the integration's own API token (apiToken) when it has one; a GitHub
//     Enterprise Server project syncs with https://<host>/api/v3 and needs
//     it;
//   - otherwise the operator's credentials, but only in the default tenant
//     (KI-85): the Plane token (plane.api_token) and the GitHub token
//     (github.token) are the operator's, another tenant's integration needs
//     its own token.
//     GitLab without a token syncs anonymously (public projects).
//
// The operator's Plane token goes only to the operator's Plane (S3-F
// security review S3): a project whose plane_base_url names another host is
// refused. GitLab's base URL is the project's repository host.
func (s *PMWebhookService) providerConfig(ctx context.Context, provider string, proj *project.Project, apiToken string) (map[string]string, error) {
	operator := operatorCredentialsServe(ctx)
	switch provider {
	case "gitlab":
		base, ok := repoBaseURL(proj.RepoURL)
		if !ok {
			return nil, fmt.Errorf("gitlab webhook: project %s has no GitLab repository URL: %w", proj.ID, domain.ErrValidation)
		}
		return map[string]string{"base_url": base, "token": apiToken}, nil
	case "github-issues":
		// A GitHub Enterprise Server project syncs with its server's API and
		// the integration's own token: github.token is for api.github.com
		// only (KI-166).
		if base, ok := repoBaseURL(proj.RepoURL); ok && !isGitHubDotCom(base) {
			if apiToken == "" {
				return nil, fmt.Errorf("github webhook: project %s is on %s: the integration needs its own api_token (github.token is for api.github.com only): %w",
					proj.ID, base, domain.ErrValidation)
			}
			return map[string]string{"base_url": base + "/api/v3", "token": apiToken}, nil
		}
		if apiToken == "" && !operator {
			return nil, fmt.Errorf("github webhook: project %s: the integration has no api_token, and github.token serves only the default tenant: %w",
				proj.ID, domain.ErrValidation)
		}
		return map[string]string{"token": apiToken}, nil
	case "plane":
		cfg := maps.Clone(s.providerConfigs[provider])
		if cfg == nil {
			cfg = map[string]string{}
		}
		if own := proj.Config["plane_base_url"]; own != "" && !sameBaseURL(own, cfg["base_url"]) {
			// The operator's Plane may be an internal host: it is logged,
			// never named to the tenant (the answer, the pm.sync event).
			slog.WarnContext(ctx, "plane webhook: the project's plane_base_url is not the operator's Plane - not synced",
				"project_id", proj.ID, "plane_base_url", own, "operator_plane_base_url", cfg["base_url"])
			return nil, fmt.Errorf("plane webhook: project %s sets plane_base_url %q, which is not the operator's Plane "+
				"(plane.base_url) - the sync is not sent there: %w", proj.ID, own, domain.ErrValidation)
		}
		switch {
		case apiToken != "":
			cfg["api_token"] = apiToken
		case !operator:
			return nil, fmt.Errorf("plane webhook: project %s: the integration has no api_token, and the operator's Plane token serves only the default tenant: %w",
				proj.ID, domain.ErrValidation)
		}
		return cfg, nil
	}
	return nil, fmt.Errorf("webhook provider %q: %w", provider, domain.ErrValidation)
}

// isGitHubDotCom reports whether base (scheme://host[:port]) is github.com.
func isGitHubDotCom(base string) bool {
	u, err := neturl.Parse(base)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return host == "github.com" || host == "www.github.com"
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

// webhookHost returns the host of a repository URL from a webhook payload,
// or fallback when the payload has none.
func webhookHost(repoURL, fallback string) string {
	if u, err := neturl.Parse(repoURL); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return fallback
}
