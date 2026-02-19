package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
)

// VCSWebhookService processes VCS webhook events from GitHub and GitLab.
type VCSWebhookService struct {
	hub broadcast.Broadcaster
}

// NewVCSWebhookService creates a new VCSWebhookService.
func NewVCSWebhookService(hub broadcast.Broadcaster) *VCSWebhookService {
	return &VCSWebhookService{hub: hub}
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

	branch := extractBranchFromRef(raw.Ref)

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
			Provider:   "github",
			Repository: raw.Repository.FullName,
			Branch:     branch,
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
	ev.FileCount = countFiles(ev.Commits)

	slog.Info("github push event", "repo", ev.Repository, "branch", branch, "commits", len(ev.Commits))

	s.hub.BroadcastEvent(ctx, ws.EventVCSPush, ev)
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

	branch := extractBranchFromRef(raw.Ref)

	ev := &webhook.VCSPushEvent{
		VCSEvent: webhook.VCSEvent{
			Type:       webhook.VCSEventPush,
			Provider:   "gitlab",
			Repository: raw.Project.PathWithNamespace,
			Branch:     branch,
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
	ev.FileCount = countFiles(ev.Commits)

	slog.Info("gitlab push event", "repo", ev.Repository, "branch", branch, "commits", len(ev.Commits))

	s.hub.BroadcastEvent(ctx, ws.EventVCSPush, ev)
	return ev, nil
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

	s.hub.BroadcastEvent(ctx, ws.EventVCSPullRequest, ev)
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
