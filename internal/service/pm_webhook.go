package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// PMWebhookService processes PM platform webhooks and triggers sync.
type PMWebhookService struct {
	hub   broadcast.Broadcaster
	sync  *SyncService
	store database.Store
}

// NewPMWebhookService creates a PM webhook service.
func NewPMWebhookService(hub broadcast.Broadcaster, syncSvc *SyncService, store database.Store) *PMWebhookService {
	return &PMWebhookService{hub: hub, sync: syncSvc, store: store}
}

// triggerPullSync looks up the project by repo name and triggers a pull sync.
// Runs in a goroutine — errors are logged, not returned.
func (s *PMWebhookService) triggerPullSync(ctx context.Context, provider, projectRef string) {
	proj, err := s.store.GetProjectByRepoName(ctx, projectRef)
	if err != nil {
		slog.Warn("webhook: project not found for sync", "provider", provider, "ref", projectRef, "error", err)
		return
	}

	cfg := roadmap.SyncConfig{
		ProjectID:   proj.ID,
		ProjectRef:  projectRef,
		Provider:    provider,
		Direction:   roadmap.SyncDirectionPull,
		CreateNew:   true,
		UpdateExist: true,
	}

	result, syncErr := s.sync.Sync(ctx, cfg)
	if syncErr != nil {
		slog.Error("webhook: pull sync failed", "provider", provider, "project", proj.ID, "error", syncErr)
		return
	}

	slog.Info("webhook: pull sync completed",
		"provider", provider,
		"project", proj.ID,
		"created", result.Created,
		"updated", result.Updated,
	)

	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, "pm.sync", map[string]any{
			"project_id": proj.ID,
			"provider":   provider,
			"created":    result.Created,
			"updated":    result.Updated,
		})
	}
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
		return nil, fmt.Errorf("parse github issue webhook: %w", err)
	}

	evt := &webhook.PMWebhookEvent{
		Provider:   "github",
		Action:     raw.Action,
		ItemID:     fmt.Sprintf("%d", raw.Issue.Number),
		ProjectRef: raw.Repository.FullName,
	}

	slog.Info("github issue webhook received",
		"action", evt.Action,
		"item_id", evt.ItemID,
		"project_ref", evt.ProjectRef,
	)

	go s.triggerPullSync(ctx, evt.Provider, evt.ProjectRef)

	return evt, nil
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
		return nil, fmt.Errorf("parse gitlab issue webhook: %w", err)
	}

	evt := &webhook.PMWebhookEvent{
		Provider:   "gitlab",
		Action:     raw.ObjectAttributes.Action,
		ItemID:     fmt.Sprintf("%d", raw.ObjectAttributes.IID),
		ProjectRef: raw.Project.PathWithNamespace,
	}

	slog.Info("gitlab issue webhook received",
		"action", evt.Action,
		"item_id", evt.ItemID,
		"project_ref", evt.ProjectRef,
	)

	go s.triggerPullSync(ctx, evt.Provider, evt.ProjectRef)

	return evt, nil
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
		return nil, fmt.Errorf("parse plane webhook: %w", err)
	}

	// Plane events are like "issue.created", "issue.updated"
	parts := strings.SplitN(raw.Event, ".", 2)
	action := raw.Event
	if len(parts) == 2 {
		action = parts[1]
	}

	evt := &webhook.PMWebhookEvent{
		Provider:   "plane",
		Action:     action,
		ItemID:     raw.Data.ID,
		ProjectRef: fmt.Sprintf("%s/%s", raw.Data.Workspace, raw.Data.Project),
	}

	slog.Info("plane webhook received",
		"action", evt.Action,
		"item_id", evt.ItemID,
		"project_ref", evt.ProjectRef,
	)

	go s.triggerPullSync(ctx, evt.Provider, evt.ProjectRef)

	return evt, nil
}
