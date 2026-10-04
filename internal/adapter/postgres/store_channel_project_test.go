package postgres_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-89 review (pre-existing): CreateChannel took project_id from the request
// body without a tenant check, so a tenant could attach a channel to another
// tenant's project (its project channel list, cascade on that project's
// deletion). A channel is created only without a project or for a project of
// the caller's tenant; another tenant's project is not found.
func TestStore_CreateChannel_ProjectOfTheTenant(t *testing.T) {
	store := setupStore(t)
	ctxA := ctxWithTenant(t, createTestTenant(t, store))
	ctxB := ctxWithTenant(t, createTestTenant(t, store))
	projA, err := store.CreateProject(ctxA, &project.CreateRequest{Name: "chan-" + uuid.NewString()[:8], Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctxA, projA.ID) })

	create := func(ctx context.Context, projectID string) (*channel.Channel, error) {
		ch, err := store.CreateChannel(ctx, &channel.Channel{
			Name: "chan-" + uuid.NewString()[:8], Type: channel.TypeProject, ProjectID: projectID,
		})
		if err == nil {
			t.Cleanup(func() { _ = store.DeleteChannel(ctx, ch.ID) })
		}
		return ch, err
	}
	if ch, err := create(ctxB, projA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tenant B attaches a channel to tenant A's project = %+v, %v; want ErrNotFound", ch, err)
	}
	if ch, err := create(ctxB, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("channel for an unknown project = %+v, %v; want ErrNotFound", ch, err)
	}
	if ch, err := create(ctxA, projA.ID); err != nil || ch.ProjectID != projA.ID {
		t.Fatalf("channel for the tenant's own project = %+v, %v", ch, err)
	}
	if ch, err := create(ctxB, ""); err != nil || ch.ProjectID != "" {
		t.Fatalf("channel without a project = %+v, %v", ch, err)
	}
	if list, err := store.ListChannels(ctxA, projA.ID, ""); err != nil || len(list) != 1 {
		t.Fatalf("tenant A's project channels = %d, %v; want only its own", len(list), err)
	}

	// Through the API: 404.
	r := chi.NewRouter()
	r.Use(middleware.TenantID)
	cfhttp.MountRoutes(r, &cfhttp.Handlers{
		Channels: service.NewChannelService(store, noopBroadcaster{}),
		Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
	})
	editorB := &user.User{ID: user.AuthDisabledUserID, Name: "B", Role: user.RoleEditor, TenantID: middleware.TenantIDFromContext(ctxB)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels",
		strings.NewReader(`{"name":"chan-x","type":"project","project_id":"`+projA.ID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(middleware.ContextWithTestUser(context.Background(), editorB)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /channels with another tenant's project: status %d (%s), want 404", w.Code, w.Body.String())
	}
}
