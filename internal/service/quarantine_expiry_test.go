package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-91: quarantine_messages has expires_at and a status "expired", but
// nothing set it, so a held message - and the inbound A2A task waiting for
// it - waited until an admin decided. A periodic sweep now expires overdue
// messages in their tenant; a held A2A task is rejected with its message in
// one step (the store's transaction), as Reject does.

// expiryStore expires messages like the Postgres store: only a pending,
// overdue message changes, and the held A2A task is rejected with it only
// while it waits for that message.
type expiryStore struct {
	*a2aQuarantineStore
	now       time.Time
	calls     []string // "<tenant>/<message>/<held task>"
	reviews   []quarantine.Review
	expireErr map[string]error
	// listed, if set, is what the listing returns (a listing taken before
	// a decision).
	listed []*quarantine.Message
}

func (s *expiryStore) ListExpiredQuarantineMessages(_ context.Context, limit int) ([]*quarantine.Message, error) {
	if s.listed != nil {
		return s.listed, nil
	}
	var due []*quarantine.Message
	for _, m := range s.messages {
		if m.Status == quarantine.StatusPending && !m.ExpiresAt.After(s.now) {
			cp := *m
			due = append(due, &cp)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].ExpiresAt.Before(due[j].ExpiresAt) })
	if len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (s *expiryStore) ExpireQuarantineMessage(ctx context.Context, id, heldTaskID string, review *quarantine.Review) (database.QuarantineExpiry, error) {
	s.calls = append(s.calls, tenantctx.FromContext(ctx)+"/"+id+"/"+heldTaskID)
	s.reviews = append(s.reviews, *review)
	if err := s.expireErr[id]; err != nil {
		return database.QuarantineExpiry{}, err
	}
	m, ok := s.messages[id]
	if !ok || m.TenantID != tenantctx.FromContext(ctx) || m.Status != quarantine.StatusPending || m.ExpiresAt.After(s.now) {
		return database.QuarantineExpiry{}, nil
	}
	m.Status = quarantine.StatusExpired
	res := database.QuarantineExpiry{Expired: true}
	if t, ok := s.a2aTasks[heldTaskID]; ok && t.State == a2adomain.TaskStateSubmitted &&
		(t.Metadata[a2adomain.MetadataQuarantineMessageID] == id || t.Metadata[a2adomain.MetadataQuarantineMessageID] == "") {
		t.State = a2adomain.TaskStateRejected
		res.RejectedTaskID, res.RejectedTaskDirection = t.ID, string(t.Direction)
	}
	return res, nil
}

// expiryEvent is a broadcast event with the tenant of its context.
type expiryEvent struct {
	tenant, eventType string
	payload           any
}

type expiryBroadcaster struct{ events []expiryEvent }

func (b *expiryBroadcaster) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	b.events = append(b.events, expiryEvent{tenantctx.FromContext(ctx), eventType, payload})
}

const (
	expiryTenantA = "11111111-1111-1111-1111-111111111111"
	expiryTenantB = "22222222-2222-2222-2222-222222222222"
)

// expiryEnv holds, as of now:
//   - q-a2a (tenant A): overdue, the held prompt of the waiting inbound task a2a-held
//   - q-run (tenant B): overdue, a held run start (no A2A task)
//   - q-later (tenant A): pending, not yet due
//   - q-done (tenant B): overdue but already approved
func expiryEnv(t *testing.T) (*expiryStore, *expiryBroadcaster, *QuarantineService) {
	t.Helper()
	now := time.Now()
	payload, err := json.Marshal(messagequeue.A2ATaskCreatedPayload{TaskID: "a2a-held", TenantID: expiryTenantA, Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	store := &expiryStore{
		a2aQuarantineStore: &a2aQuarantineStore{mockQuarantineStore: newMockQuarantineStore(), a2aTasks: map[string]*a2adomain.A2ATask{}},
		now:                now,
		expireErr:          map[string]error{},
	}
	add := func(id, tenant, subject string, payload []byte, status quarantine.Status, expires time.Time) {
		store.messages[id] = &quarantine.Message{ID: id, TenantID: tenant, Subject: subject, Payload: payload, Status: status, ExpiresAt: expires}
	}
	add("q-a2a", expiryTenantA, messagequeue.SubjectA2ATaskCreated, payload, quarantine.StatusPending, now.Add(-2*time.Hour))
	add("q-run", expiryTenantB, "runs.start", []byte(`{}`), quarantine.StatusPending, now.Add(-time.Hour))
	add("q-later", expiryTenantA, "runs.start", []byte(`{}`), quarantine.StatusPending, now.Add(time.Hour))
	add("q-done", expiryTenantB, "runs.start", []byte(`{}`), quarantine.StatusApproved, now.Add(-3*time.Hour))
	task := a2adomain.NewA2ATask("a2a-held")
	task.State = a2adomain.TaskStateSubmitted
	task.Direction = a2adomain.DirectionInbound
	task.Metadata[a2adomain.MetadataQuarantineMessageID] = "q-a2a"
	store.a2aTasks[task.ID] = task
	hub := &expiryBroadcaster{}
	return store, hub, NewQuarantineService(store, &mockQueue{}, hub, config.Quarantine{Enabled: true})
}

func TestQuarantine_ExpireOverdue(t *testing.T) {
	store, hub, svc := expiryEnv(t)

	n, err := svc.ExpireOverdue(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("ExpireOverdue = %d, %v; want 2 expired", n, err)
	}
	// Each message is expired in its own tenant, the A2A prompt with the
	// task its payload names.
	want := []string{expiryTenantA + "/q-a2a/a2a-held", expiryTenantB + "/q-run/"}
	if !slices.Equal(store.calls, want) {
		t.Fatalf("expire calls = %v, want %v", store.calls, want)
	}
	for _, r := range store.reviews {
		if r.ReviewerID != "" || r.ReviewerName == "" || !strings.Contains(r.Note, "expired") {
			t.Errorf("expiry review = %+v, want no user, a reviewer name and a note saying it expired", r)
		}
	}
	for id, want := range map[string]quarantine.Status{
		"q-a2a": quarantine.StatusExpired, "q-run": quarantine.StatusExpired,
		"q-later": quarantine.StatusPending, "q-done": quarantine.StatusApproved,
	} {
		if got := store.messages[id].Status; got != want {
			t.Errorf("%s status = %s, want %s", id, got, want)
		}
	}
	if got := store.a2aTasks["a2a-held"].State; got != a2adomain.TaskStateRejected {
		t.Errorf("held a2a task = %s, want rejected", got)
	}

	// As Reject: the task's status and the message's resolution are
	// announced, each to its tenant only.
	var got []string
	for _, ev := range hub.events {
		switch p := ev.payload.(type) {
		case map[string]string:
			got = append(got, ev.tenant+" "+ev.eventType+" "+p["task_id"]+" "+p["state"]+" "+p["direction"])
		case event.QuarantineResolvedEvent:
			got = append(got, ev.tenant+" "+ev.eventType+" "+p.ID+" "+p.Action)
		default:
			t.Errorf("unexpected event %s %#v", ev.eventType, ev.payload)
		}
	}
	wantEvents := []string{
		expiryTenantA + " " + event.EventA2ATaskStatus + " a2a-held rejected inbound",
		expiryTenantA + " " + event.EventQuarantineResolved + " q-a2a expired",
		expiryTenantB + " " + event.EventQuarantineResolved + " q-run expired",
	}
	if !slices.Equal(got, wantEvents) {
		t.Errorf("events = %v, want %v", got, wantEvents)
	}

	// Idempotent: a second sweep finds nothing to do.
	store.calls = nil
	if n, err := svc.ExpireOverdue(context.Background()); err != nil || n != 0 || len(store.calls) != 0 {
		t.Fatalf("second sweep = %d, %v (calls %v); want nothing", n, err, store.calls)
	}
}

// A held A2A task that no longer waits for its message (its caller
// cancelled it, or it names another message) is left alone; the message
// still expires.
func TestQuarantine_ExpireOverdue_TaskNoLongerWaiting(t *testing.T) {
	for name, modify := range map[string]func(*a2adomain.A2ATask){
		"cancelled":           func(task *a2adomain.A2ATask) { task.State = a2adomain.TaskStateCanceled },
		"names other message": func(task *a2adomain.A2ATask) { task.Metadata[a2adomain.MetadataQuarantineMessageID] = "q-other" },
	} {
		t.Run(name, func(t *testing.T) {
			store, hub, svc := expiryEnv(t)
			modify(store.a2aTasks["a2a-held"])
			before := store.a2aTasks["a2a-held"].State

			if _, err := svc.ExpireOverdue(context.Background()); err != nil {
				t.Fatalf("ExpireOverdue: %v", err)
			}
			if got := store.messages["q-a2a"].Status; got != quarantine.StatusExpired {
				t.Errorf("message = %s, want expired", got)
			}
			if got := store.a2aTasks["a2a-held"].State; got != before {
				t.Errorf("task = %s, want unchanged %s", got, before)
			}
			for _, ev := range hub.events {
				if ev.eventType == event.EventA2ATaskStatus {
					t.Errorf("task status announced for a task that did not change: %+v", ev)
				}
			}
		})
	}
}

// A message an admin (or another replica's sweep) decided after it was
// listed is not expired, counted or announced; a failing message does not
// stop the others and is reported.
func TestQuarantine_ExpireOverdue_DecidedMeanwhileAndErrors(t *testing.T) {
	store, hub, svc := expiryEnv(t)
	listed, err := store.ListExpiredQuarantineMessages(context.Background(), quarantineExpiryBatch)
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed = %d, %v", len(listed), err)
	}
	// The admin rejects q-a2a between the listing and the expiry.
	store.listed = listed
	store.messages["q-a2a"].Status = quarantine.StatusRejected
	store.expireErr["q-run"] = errors.New("db down")

	n, err := svc.ExpireOverdue(context.Background())
	if n != 0 || err == nil || !strings.Contains(err.Error(), "q-run") {
		t.Fatalf("ExpireOverdue = %d, %v; want 0 and the q-run error", n, err)
	}
	if got := store.a2aTasks["a2a-held"].State; got != a2adomain.TaskStateSubmitted {
		t.Errorf("task = %s, want unchanged (the admin's decision handles it)", got)
	}
	if len(store.calls) != 2 {
		t.Errorf("expire calls = %v, want both messages tried", store.calls)
	}
	if len(hub.events) != 0 {
		t.Errorf("events = %+v, want none", hub.events)
	}
}
