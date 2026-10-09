package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
)

// VCSWebhookService processes VCS webhook events from GitHub and GitLab for
// the project of the webhook they arrived on (KI-85).
type VCSWebhookService struct {
	hub    broadcast.Broadcaster
	review *ReviewService
}

// NewVCSWebhookService creates a new VCSWebhookService.
func NewVCSWebhookService(hub broadcast.Broadcaster) *VCSWebhookService {
	return &VCSWebhookService{hub: hub}
}

// SetReviewService sets the review service for triggering automated reviews on push events.
func (s *VCSWebhookService) SetReviewService(rs *ReviewService) {
	s.review = rs
}

// webhookRepository is the repository a GitHub or GitLab payload names.
type webhookRepository struct {
	fullName string // GitHub full_name, GitLab path_with_namespace
	webURL   string // GitHub html_url, GitLab web_url (its host)
}

// matches reports whether the payload's repository is the project's: the
// same host and full path (GitLab subgroups included), case-insensitive as
// GitHub and GitLab treat them, never a part of it (KI-85, D3). A payload
// without a URL is on the provider's public host.
func (r webhookRepository) matches(proj *project.Project, defaultHost string) bool {
	host, path, ok := project.RepoHostPath(proj.RepoURL)
	return ok && r.fullName != "" &&
		strings.EqualFold(host, webhookHost(r.webURL, defaultHost)) && strings.EqualFold(path, r.fullName)
}

// checkRepository refuses an event for another repository than the
// webhook's project with webhook.ErrRepositoryMismatch (the event is
// ignored and logged).
func checkRepository(proj *project.Project, provider string, repo webhookRepository) error {
	defaultHost := "github.com"
	if provider == "gitlab" {
		defaultHost = "gitlab.com"
	}
	if repo.matches(proj, defaultHost) {
		return nil
	}
	slog.Warn("webhook event for another repository ignored",
		"provider", provider, "project_id", proj.ID, "event_repository", repo.fullName, "event_url", repo.webURL)
	return fmt.Errorf("%s event for %q, project %s is %q: %w", provider, repo.fullName, proj.ID, proj.RepoURL, webhook.ErrRepositoryMismatch)
}

// parsePayload decodes a webhook payload; a payload that cannot be read is
// a validation error (400).
func parsePayload[T any](data []byte, what string, v *T) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w: %w", what, err, domain.ErrValidation)
	}
	return nil
}

// HandleGitHubPush processes a GitHub push webhook payload for proj.
func (s *VCSWebhookService) HandleGitHubPush(ctx context.Context, proj *project.Project, data []byte) (*webhook.VCSPushEvent, error) {
	var raw struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Forced     bool   `json:"forced"`
		Repository struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
		Commits []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
		} `json:"commits"`
	}
	if err := parsePayload(data, "github push", &raw); err != nil {
		return nil, err
	}
	if err := checkRepository(proj, "github", webhookRepository{raw.Repository.FullName, raw.Repository.HTMLURL}); err != nil {
		return nil, err
	}

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
			ProjectID:  proj.ID,
			Provider:   "github",
			Repository: raw.Repository.FullName,
			Branch:     extractBranchFromRef(raw.Ref),
			Sender:     raw.Sender.Login,
			CommitHash: raw.After,
		},
		Before: raw.Before,
		After:  raw.After,
		Forced: raw.Forced,
	}

	for _, c := range raw.Commits {
		ev.Commits = append(ev.Commits, webhook.VCSCommit{
			Hash:     c.ID,
			Message:  c.Message,
			Author:   c.Author.Name,
			Added:    c.Added,
			Modified: c.Modified,
			Removed:  c.Removed,
		})
	}

	s.processPushEvent(ctx, ev)
	return ev, nil
}

// HandleGitLabPush processes a GitLab push webhook payload for proj.
func (s *VCSWebhookService) HandleGitLabPush(ctx context.Context, proj *project.Project, data []byte) (*webhook.VCSPushEvent, error) {
	var raw struct {
		Ref     string `json:"ref"`
		Before  string `json:"before"`
		After   string `json:"after"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
			WebURL            string `json:"web_url"`
		} `json:"project"`
		UserUsername string `json:"user_username"`
		Commits      []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
		} `json:"commits"`
	}
	if err := parsePayload(data, "gitlab push", &raw); err != nil {
		return nil, err
	}
	if err := checkRepository(proj, "gitlab", webhookRepository{raw.Project.PathWithNamespace, raw.Project.WebURL}); err != nil {
		return nil, err
	}

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
			ProjectID:  proj.ID,
			Provider:   "gitlab",
			Repository: raw.Project.PathWithNamespace,
			Branch:     extractBranchFromRef(raw.Ref),
			Sender:     raw.UserUsername,
			CommitHash: raw.After,
		},
		Before: raw.Before,
		After:  raw.After,
	}

	for _, c := range raw.Commits {
		ev.Commits = append(ev.Commits, webhook.VCSCommit{
			Hash:     c.ID,
			Message:  c.Message,
			Author:   c.Author.Name,
			Added:    c.Added,
			Modified: c.Modified,
			Removed:  c.Removed,
		})
	}

	s.processPushEvent(ctx, ev)
	return ev, nil
}

// send broadcasts an event to the webhook's tenant.
func (s *VCSWebhookService) send(ctx context.Context, eventType string, payload any) {
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, eventType, payload)
	}
}

// processPushEvent handles the shared post-parse logic for push events:
// file counting, logging, broadcasting, and triggering review checks.
func (s *VCSWebhookService) processPushEvent(ctx context.Context, ev *webhook.VCSPushEvent) {
	ev.FileCount = countFiles(ev.Commits)

	slog.Info("VCS push received",
		"provider", ev.Provider,
		"project_id", ev.ProjectID,
		"repo", ev.Repository,
		"branch", ev.Branch,
		"commits", len(ev.Commits),
	)

	s.send(ctx, event.EventVCSPush, ev)

	if s.review != nil {
		if pushErr := s.review.HandlePush(ctx, ev.ProjectID, ev.Branch, len(ev.Commits)); pushErr != nil {
			slog.Warn("review trigger failed",
				"project_id", ev.ProjectID,
				"branch", ev.Branch,
				"error", pushErr,
			)
		}
	}
}

// HandleGitHubPullRequest processes a GitHub pull_request webhook payload
// for proj.
func (s *VCSWebhookService) HandleGitHubPullRequest(ctx context.Context, proj *project.Project, data []byte) (*webhook.VCSPullRequestEvent, error) {
	var raw struct {
		Action      string `json:"action"`
		PullRequest struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Draft  bool   `json:"draft"`
			Head   struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := parsePayload(data, "github pull_request", &raw); err != nil {
		return nil, err
	}
	if err := checkRepository(proj, "github", webhookRepository{raw.Repository.FullName, raw.Repository.HTMLURL}); err != nil {
		return nil, err
	}

	ev := &webhook.VCSPullRequestEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPullRequest,
			ProjectID:  proj.ID,
			Provider:   "github",
			Repository: raw.Repository.FullName,
			Branch:     raw.PullRequest.Head.Ref,
			Sender:     raw.Sender.Login,
			CommitHash: raw.PullRequest.Head.SHA,
		},
		Action:     raw.Action,
		PRNumber:   raw.PullRequest.Number,
		Title:      raw.PullRequest.Title,
		BaseBranch: raw.PullRequest.Base.Ref,
		HeadBranch: raw.PullRequest.Head.Ref,
		Draft:      raw.PullRequest.Draft,
	}

	slog.Info("github PR event", "project_id", proj.ID, "repo", ev.Repository, "action", ev.Action, "pr", ev.PRNumber)

	s.send(ctx, event.EventVCSPullRequest, ev)

	// Trigger pre-merge review checks on PR open/synchronize.
	if s.review != nil && (ev.Action == "opened" || ev.Action == "synchronize") {
		if _, prErr := s.review.HandlePreMerge(ctx, proj.ID, ev.BaseBranch); prErr != nil {
			slog.Warn("pre-merge review trigger failed",
				"project_id", proj.ID,
				"base_branch", ev.BaseBranch,
				"error", prErr,
			)
		}
	}

	return ev, nil
}

func extractBranchFromRef(ref string) string {
	// refs/heads/main -> main, refs/heads/feature/foo -> feature/foo
	const prefix = "refs/heads/"
	if strings.HasPrefix(ref, prefix) {
		return ref[len(prefix):]
	}
	return ref
}

func countFiles(commits []webhook.VCSCommit) int {
	seen := make(map[string]struct{})
	for _, c := range commits {
		for _, f := range c.Added {
			seen[f] = struct{}{}
		}
		for _, f := range c.Modified {
			seen[f] = struct{}{}
		}
		for _, f := range c.Removed {
			seen[f] = struct{}{}
		}
	}
	return len(seen)
}
