package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// quarantineListStore records which project's quarantined messages are listed.
type quarantineListStore struct {
	database.Store
	projects []string
}

func (s *quarantineListStore) ListQuarantinedMessages(_ context.Context, projectID string, _ quarantine.Status, _, _ int) ([]*quarantine.Message, error) {
	s.projects = append(s.projects, projectID)
	return []*quarantine.Message{{ID: "q-1", Subject: "a2a.task.created"}}, nil
}

// TestQuarantineList_WithoutProject (KI-15): inbound A2A prompts belong to
// no project; their quarantined messages are listed (and counted) when no
// project_id is given, which used to be refused with 400.
func TestQuarantineList_WithoutProject(t *testing.T) {
	for name, serve := range map[string]func(h *Handlers, w http.ResponseWriter, r *http.Request){
		"list":  (*Handlers).listQuarantinedMessages,
		"stats": (*Handlers).quarantineStats,
	} {
		t.Run(name, func(t *testing.T) {
			store := &quarantineListStore{}
			h := &Handlers{Quarantine: service.NewQuarantineService(store, nil, nil, config.Quarantine{Enabled: true})}
			rec := httptest.NewRecorder()
			serve(h, rec, httptest.NewRequest(http.MethodGet, "/api/v1/quarantine", http.NoBody))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
			}
			if len(store.projects) == 0 {
				t.Fatal("no messages listed")
			}
			for _, p := range store.projects {
				if p != "" {
					t.Errorf("listed project %q, want the messages without project", p)
				}
			}
		})
	}
}
