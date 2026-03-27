package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// VCSWebhookService processes VCS webhook events from GitHub and GitLab.
type VCSWebhookService struct {
	hub    broadcast.Broadcaster
	store  database.Store
	review *ReviewService
}

// NewVCSWebhookService creates a new VCSWebhookService.
func NewVCSWebhookService(hub broadcast.Broadcaster, store database.Store) *VCSWebhookService {
	return &VCSWebhookService{hub: hub, store: store}
}

// SetReviewService sets the review service for triggering automated reviews on push events.
func (s *VCSWebhookService) SetReviewService(rs *ReviewService) {
	s.review = rs
}

// resolveProject looks up a project ID by repository name.
func (s *VCSWebhookService) resolveProject(ctx context.Context, repoFullName string) (string, error) {
	p, err := s.store.GetProjectByRepoName(ctx, repoFullName)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

// HandleGitHubPush processes a GitHub push webhook payload.
func (s *VCSWebhookService) HandleGitHubPush(ctx context.Context, data []byte) (*webhook.VCSPushEvent, error) {
	var raw struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Forced     bool   `json:"forced"`
		Repository struct {
			FullName string `json:"full_name"`
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
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse github push: %w", err)
	}

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
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

// HandleGitLabPush processes a GitLab push webhook payload.
func (s *VCSWebhookService) HandleGitLabPush(ctx context.Context, data []byte) (*webhook.VCSPushEvent, error) {
	var raw struct {
		Ref     string `json:"ref"`
		Before  string `json:"before"`
		After   string `json:"after"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
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
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gitlab push: %w", err)
	}

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
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

// processPushEvent handles the shared post-parse logic for push events:
// file counting, logging, broadcasting, and triggering review checks.
func (s *VCSWebhookService) processPushEvent(ctx context.Context, ev *webhook.VCSPushEvent) {
	ev.FileCount = countFiles(ev.Commits)

	slog.Info("VCS push received",
		"provider", ev.Provider,
		"repo", ev.Repository,
		"branch", ev.Branch,
		"commits", len(ev.Commits),
	)

	s.hub.BroadcastEvent(ctx, event.EventVCSPush, ev)

	if s.review != nil && s.store != nil {
		if projectID, err := s.resolveProject(ctx, ev.Repository); err == nil {
			if pushErr := s.review.HandlePush(ctx, projectID, ev.Branch, len(ev.Commits)); pushErr != nil {
				slog.Warn("review trigger failed",
					"project_id", projectID,
					"branch", ev.Branch,
					"error", pushErr,
				)
			}
		}
	}
}

// HandleGitHubPullRequest processes a GitHub pull_request webhook payload.
func (s *VCSWebhookService) HandleGitHubPullRequest(ctx context.Context, data []byte) (*webhook.VCSPullRequestEvent, error) {
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
		} `json:"repository"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse github pull_request: %w", err)
	}

	ev := &webhook.VCSPullRequestEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPullRequest,
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

	slog.Info("github PR event", "repo", ev.Repository, "action", ev.Action, "pr", ev.PRNumber)

	s.hub.BroadcastEvent(ctx, event.EventVCSPullRequest, ev)

	// Trigger pre-merge review checks on PR open/synchronize.
	if s.review != nil && s.store != nil && (ev.Action == "opened" || ev.Action == "synchronize") {
		if projectID, err := s.resolveProject(ctx, ev.Repository); err == nil {
			if _, prErr := s.review.HandlePreMerge(ctx, projectID, ev.BaseBranch); prErr != nil {
				slog.Warn("pre-merge review trigger failed",
					"project_id", projectID,
					"base_branch", ev.BaseBranch,
					"error", prErr,
				)
			}
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
