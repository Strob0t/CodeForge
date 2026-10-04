package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/service"
)

// quarantineHTTPStore keeps one pending quarantined message and records its review.
type quarantineHTTPStore struct {
	*mockStore
	msg     quarantine.Message
	reviews []quarantine.Review
}

func (s *quarantineHTTPStore) GetQuarantinedMessage(_ context.Context, id string) (*quarantine.Message, error) {
	if id != s.msg.ID {
		return nil, domain.ErrNotFound
	}
	msg := s.msg
	return &msg, nil
}

func (s *quarantineHTTPStore) UpdateQuarantineStatus(_ context.Context, id string, status quarantine.Status, review *quarantine.Review) error {
	if id != s.msg.ID {
		return domain.ErrNotFound
	}
	s.msg.Status = status
	s.reviews = append(s.reviews, *review)
	return nil
}

// KI-79: the reviewer of a quarantined message is the logged-in user (ID and
// name at the time), not a name typed into the request.
func TestQuarantineReview_RecordsTheLoggedInReviewer(t *testing.T) {
	for _, action := range []string{"approve", "reject"} {
		t.Run(action, func(t *testing.T) {
			store := &quarantineHTTPStore{
				mockStore: &mockStore{},
				msg:       quarantine.Message{ID: "q-1", ProjectID: "proj-1", Subject: "runs.start", Status: quarantine.StatusPending},
			}
			router := newTestRouterWithLLM(store.mockStore, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
				func(h *cfhttp.Handlers) {
					h.Quarantine = service.NewQuarantineService(store, &mockQueue{}, &mockBroadcaster{}, config.Quarantine{Enabled: true})
				})
			admin := &user.User{ID: "user-7", Name: "Ada Admin", Email: "ada@example.com", Role: user.RoleAdmin, TenantID: channelTestTenant}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, channelRouteRequest(http.MethodPost, "/api/v1/quarantine/q-1/"+action,
				`{"reviewed_by":"someone else","note":"checked"}`, admin))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s = %d: %s", action, rec.Code, rec.Body.String())
			}
			want := quarantine.Review{ReviewerID: "user-7", ReviewerName: "Ada Admin", Note: "checked"}
			if len(store.reviews) != 1 || store.reviews[0] != want {
				t.Fatalf("reviews = %+v, want [%+v]", store.reviews, want)
			}
		})
	}
}
