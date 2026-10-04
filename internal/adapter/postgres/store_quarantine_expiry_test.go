package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-91: quarantine messages past expires_at were never expired, so a held
// inbound A2A task waited until an admin decided. The store expires an
// overdue pending message and rejects the A2A task waiting for it in one
// transaction; both changes are conditional, so a repeated sweep, a second
// replica's sweep or an admin's decision that wins changes nothing.

// The test messages are overdue by centuries, so they come first in the
// listing (ordered by deadline) whatever else the database holds.
var longOverdue = time.Now().AddDate(-300, 0, 0)

var expiryReview = &quarantine.Review{ReviewerName: "system", Note: "expired: not reviewed before its deadline"}

// quarantined stores a message of the fixture's tenant with the given
// status and deadline.
func (f *statusFixture) quarantined(t *testing.T, subject string, payload []byte, status quarantine.Status, expiresAt time.Time) string {
	t.Helper()
	msg := &quarantine.Message{
		Subject: subject, Payload: payload, Status: status, RiskFactors: []string{"test"},
		CreatedAt: time.Now().UTC(), ExpiresAt: expiresAt,
	}
	if err := f.store.QuarantineMessage(f.ctx, msg); err != nil {
		t.Fatalf("QuarantineMessage: %v", err)
	}
	return msg.ID
}

// heldA2APrompt stores an inbound A2A task of the fixture's tenant whose
// prompt waits in the quarantine until expiresAt, as the A2A executor does.
func (f *statusFixture) heldA2APrompt(t *testing.T, expiresAt time.Time) (msgID, taskID string) {
	t.Helper()
	taskID = "a2a-" + uuid.NewString()
	payload, err := json.Marshal(messagequeue.A2ATaskCreatedPayload{
		TaskID: taskID, TenantID: middleware.TenantIDFromContext(f.ctx), Prompt: "do it",
	})
	if err != nil {
		t.Fatal(err)
	}
	msgID = f.quarantined(t, messagequeue.SubjectA2ATaskCreated, payload, quarantine.StatusPending, expiresAt)
	task := a2adomain.NewA2ATask(taskID)
	task.Metadata[a2adomain.MetadataQuarantineMessageID] = msgID
	if err := f.store.CreateA2ATask(f.ctx, task); err != nil {
		t.Fatalf("CreateA2ATask: %v", err)
	}
	t.Cleanup(func() { _ = f.store.DeleteA2ATask(f.ctx, taskID) })
	return msgID, taskID
}

func (f *statusFixture) quarantineStatus(t *testing.T, id string) *quarantine.Message {
	t.Helper()
	msg, err := f.store.GetQuarantinedMessage(f.ctx, id)
	if err != nil {
		t.Fatalf("GetQuarantinedMessage: %v", err)
	}
	return msg
}

func (f *statusFixture) a2aTask(t *testing.T, id string) *a2adomain.A2ATask {
	t.Helper()
	task, err := f.store.GetA2ATask(f.ctx, id)
	if err != nil {
		t.Fatalf("GetA2ATask: %v", err)
	}
	return task
}

func TestStore_ExpireQuarantineMessage(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	runStart := []byte(`{"run":"x"}`)

	heldA, taskA := a.heldA2APrompt(t, longOverdue)
	plainB := b.quarantined(t, "runs.start", runStart, quarantine.StatusPending, longOverdue)
	notDueA := a.quarantined(t, "runs.start", runStart, quarantine.StatusPending, time.Now().Add(time.Hour))
	approvedA := a.quarantined(t, "runs.start", runStart, quarantine.StatusApproved, longOverdue)

	// The listing spans tenants (a system job): only pending, overdue
	// messages, the oldest deadline first, each with its tenant.
	listed, err := a.store.ListExpiredQuarantineMessages(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListExpiredQuarantineMessages: %v", err)
	}
	tenants := map[string]string{}
	for i := range listed {
		tenants[listed[i].ID] = listed[i].TenantID
		if i > 0 && listed[i].ExpiresAt.Before(listed[i-1].ExpiresAt) {
			t.Fatalf("listing not ordered by deadline at %d", i)
		}
	}
	if tenants[heldA] != middleware.TenantIDFromContext(a.ctx) || tenants[plainB] != middleware.TenantIDFromContext(b.ctx) {
		t.Errorf("listing = %v, want the held prompt of tenant A and the run of tenant B with their tenants", tenants)
	}
	for _, id := range []string{notDueA, approvedA} {
		if _, ok := tenants[id]; ok {
			t.Errorf("listing has %s, which is not due or not pending", id)
		}
	}
	if few, err := a.store.ListExpiredQuarantineMessages(context.Background(), 1); err != nil || len(few) != 1 {
		t.Errorf("listing with limit 1 = %d, %v", len(few), err)
	}

	// Another tenant cannot expire the message; a message not due or not
	// pending does not change.
	if res, err := b.store.ExpireQuarantineMessage(b.ctx, heldA, taskA, expiryReview); err != nil || res.Expired {
		t.Fatalf("expire from another tenant = %+v, %v; want nothing", res, err)
	}
	for _, id := range []string{notDueA, approvedA} {
		if res, err := a.store.ExpireQuarantineMessage(a.ctx, id, "", expiryReview); err != nil || res.Expired {
			t.Errorf("expire %s = %+v, %v; want nothing", id, res, err)
		}
	}
	if got := a.quarantineStatus(t, approvedA).Status; got != quarantine.StatusApproved {
		t.Errorf("approved message = %s, want unchanged", got)
	}

	// The held prompt expires and its waiting task is rejected with it.
	res, err := a.store.ExpireQuarantineMessage(a.ctx, heldA, taskA, expiryReview)
	if err != nil || !res.Expired || res.RejectedTaskID != taskA || res.RejectedTaskDirection != string(a2adomain.DirectionInbound) {
		t.Fatalf("expire held prompt = %+v, %v; want expired with its task rejected", res, err)
	}
	msg := a.quarantineStatus(t, heldA)
	if msg.Status != quarantine.StatusExpired || msg.ReviewedBy != "system" || msg.ReviewNote != expiryReview.Note ||
		msg.ReviewedByID != "" || msg.ReviewedAt == nil {
		t.Errorf("expired message = %+v", msg)
	}
	task := a.a2aTask(t, taskA)
	if task.State != a2adomain.TaskStateRejected || task.Version != 2 {
		t.Errorf("held task = %s (version %d), want rejected (version 2)", task.State, task.Version)
	}

	// Idempotent: a second expiry (a second replica) changes nothing.
	if res, err := a.store.ExpireQuarantineMessage(a.ctx, heldA, taskA, expiryReview); err != nil || res.Expired || res.RejectedTaskID != "" {
		t.Fatalf("second expiry = %+v, %v; want nothing", res, err)
	}
	if got := a.a2aTask(t, taskA).Version; got != 2 {
		t.Errorf("task version after a second expiry = %d, want 2", got)
	}

	// A message without an A2A task expires on its own.
	if res, err := b.store.ExpireQuarantineMessage(b.ctx, plainB, "", expiryReview); err != nil || !res.Expired || res.RejectedTaskID != "" {
		t.Fatalf("expire run message = %+v, %v", res, err)
	}
}

// A task that no longer waits for the message - its caller cancelled it, or
// it names another message - is left alone; the message still expires.
func TestStore_ExpireQuarantineMessage_TaskNoLongerWaiting(t *testing.T) {
	f := newStatusFixture(t)
	for name, modify := range map[string]func(task *a2adomain.A2ATask){
		"cancelled":           func(task *a2adomain.A2ATask) { task.State = a2adomain.TaskStateCanceled },
		"names other message": func(task *a2adomain.A2ATask) { task.Metadata[a2adomain.MetadataQuarantineMessageID] = uuid.NewString() },
	} {
		t.Run(name, func(t *testing.T) {
			msgID, taskID := f.heldA2APrompt(t, longOverdue)
			task := f.a2aTask(t, taskID)
			modify(task)
			if err := f.store.UpdateA2ATask(f.ctx, task); err != nil {
				t.Fatalf("UpdateA2ATask: %v", err)
			}
			res, err := f.store.ExpireQuarantineMessage(f.ctx, msgID, taskID, expiryReview)
			if err != nil || !res.Expired || res.RejectedTaskID != "" {
				t.Fatalf("expire = %+v, %v; want expired, no task rejected", res, err)
			}
			if got := f.a2aTask(t, taskID); got.State != task.State || got.Version != task.Version {
				t.Errorf("task = %s (version %d), want unchanged %s (version %d)", got.State, got.Version, task.State, task.Version)
			}
		})
	}
}

// An overdue message can no longer be approved, even before the sweep
// expired it; rejecting it is still possible.
func TestStore_UpdateQuarantineStatus_OverdueIsNotApproved(t *testing.T) {
	f := newStatusFixture(t)
	runStart := []byte(`{"run":"y"}`)
	overdue := f.quarantined(t, "runs.start", runStart, quarantine.StatusPending, time.Now().Add(-time.Minute))
	admin := &quarantine.Review{ReviewerName: "admin"}

	if err := f.store.UpdateQuarantineStatus(f.ctx, overdue, quarantine.StatusApproved, admin); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("approve overdue = %v, want ErrConflict", err)
	}
	if err := f.store.UpdateQuarantineStatus(f.ctx, overdue, quarantine.StatusRejected, admin); err != nil {
		t.Fatalf("reject overdue: %v", err)
	}
	due := f.quarantined(t, "runs.start", runStart, quarantine.StatusPending, time.Now().Add(time.Hour))
	if err := f.store.UpdateQuarantineStatus(f.ctx, due, quarantine.StatusApproved, admin); err != nil {
		t.Fatalf("approve before the deadline: %v", err)
	}
}

// An admin's decision and the expiry race for the same held prompt, with a
// second replica's sweep: exactly one wins. When a sweep wins, the message is
// expired and the task rejected once, and only that sweep reports it; when
// the admin wins, no sweep changes the message or the task.
func TestStore_ExpireQuarantineMessage_RaceWithReview(t *testing.T) {
	f := newStatusFixture(t)
	admin := &quarantine.Review{ReviewerName: "admin", Note: "no"}
	wins := map[quarantine.Status]int{}
	for range 25 {
		msgID, taskID := f.heldA2APrompt(t, longOverdue)
		var (
			wg       sync.WaitGroup
			start    = make(chan struct{})
			sweeps   [2]database.QuarantineExpiry
			sweepErr [2]error
			adminErr error
		)
		for i := range sweeps { // two replicas' sweeps
			wg.Go(func() {
				<-start
				sweeps[i], sweepErr[i] = f.store.ExpireQuarantineMessage(f.ctx, msgID, taskID, expiryReview)
			})
		}
		wg.Go(func() { // the admin's Reject
			<-start
			adminErr = f.store.UpdateQuarantineStatus(f.ctx, msgID, quarantine.StatusRejected, admin)
		})
		close(start)
		wg.Wait()
		if err := errors.Join(sweepErr[0], sweepErr[1]); err != nil {
			t.Fatalf("sweep: %v", err)
		}
		expiredBy, rejectedBy := 0, 0
		for _, s := range sweeps {
			if s.Expired {
				expiredBy++
			}
			if s.RejectedTaskID != "" {
				rejectedBy++
			}
		}

		msg, task := f.quarantineStatus(t, msgID), f.a2aTask(t, taskID)
		wins[msg.Status]++
		switch msg.Status {
		case quarantine.StatusExpired:
			if !errors.Is(adminErr, domain.ErrConflict) || expiredBy != 1 || rejectedBy != 1 {
				t.Fatalf("expiry won: admin err %v, sweeps %+v; want a conflict and one sweep that expired and rejected", adminErr, sweeps)
			}
			if task.State != a2adomain.TaskStateRejected || task.Version != 2 {
				t.Fatalf("task after the expiry = %s (version %d), want rejected once", task.State, task.Version)
			}
		case quarantine.StatusRejected:
			if adminErr != nil || msg.ReviewedBy != "admin" || expiredBy != 0 || rejectedBy != 0 {
				t.Fatalf("admin won: message %+v, admin err %v, sweeps %+v", msg, adminErr, sweeps)
			}
			if task.State != a2adomain.TaskStateSubmitted || task.Version != 1 {
				t.Fatalf("task after the admin won = %s (version %d), want untouched by the sweeps", task.State, task.Version)
			}
		default:
			t.Fatalf("message status = %s", msg.Status)
		}
	}
	t.Logf("winners: %v", wins)
}

// An admin's decision that holds the message's row when the sweep reaches
// it: the sweep waits, then sees the decision and changes nothing.
func TestStore_ExpireQuarantineMessage_WaitsForADecisionInProgress(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	msgID, taskID := f.heldA2APrompt(t, longOverdue)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE quarantine_messages SET status = 'approved', reviewed_by = 'admin', reviewed_at = now() WHERE id = $1`, msgID); err != nil {
		t.Fatalf("decision in progress: %v", err)
	}

	type outcome struct {
		expired bool
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := f.store.ExpireQuarantineMessage(f.ctx, msgID, taskID, expiryReview)
		done <- outcome{res.Expired, err}
	}()
	waitForLockWait(t, pool, "UPDATE quarantine_messages SET status = 'expired'")
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := <-done; got.err != nil || got.expired {
		t.Fatalf("expiry after the decision = %+v, want nothing changed", got)
	}
	if got := f.quarantineStatus(t, msgID).Status; got != quarantine.StatusApproved {
		t.Errorf("message = %s, want approved", got)
	}
	if got := f.a2aTask(t, taskID); got.State != a2adomain.TaskStateSubmitted || got.Version != 1 {
		t.Errorf("task = %s (version %d), want untouched", got.State, got.Version)
	}
}

// The sweep on Postgres: QuarantineService.ExpireOverdue expires each
// tenant's overdue messages in that tenant and rejects the held A2A task.
func TestQuarantine_ExpireOverdueOnPostgres(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	heldA, taskA := a.heldA2APrompt(t, longOverdue)
	plainB := b.quarantined(t, "runs.start", []byte(`{"run":"z"}`), quarantine.StatusPending, longOverdue)

	svc := service.NewQuarantineService(a.store, nil, noopBroadcaster{}, config.Quarantine{})
	n, err := svc.ExpireOverdue(context.Background())
	if err != nil || n < 2 {
		t.Fatalf("ExpireOverdue = %d, %v; want at least the 2 test messages", n, err)
	}
	if got := a.quarantineStatus(t, heldA).Status; got != quarantine.StatusExpired {
		t.Errorf("held prompt = %s, want expired", got)
	}
	if got := b.quarantineStatus(t, plainB).Status; got != quarantine.StatusExpired {
		t.Errorf("tenant B message = %s, want expired", got)
	}
	if got := a.a2aTask(t, taskA).State; got != a2adomain.TaskStateRejected {
		t.Errorf("held task = %s, want rejected", got)
	}
	listed, err := a.store.ListExpiredQuarantineMessages(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListExpiredQuarantineMessages: %v", err)
	}
	if slices.ContainsFunc(listed, func(m *quarantine.Message) bool { return m.ID == heldA || m.ID == plainB }) {
		t.Error("expired messages are still listed as due")
	}
}

func TestListExpiredQuarantineMessages_IntentionallyCrossTenant(t *testing.T) {
	src := readStoreSource(t, "store_quarantine.go")
	if doc := methodDocComment(t, src, "store_quarantine.go", "ListExpiredQuarantineMessages"); !strings.Contains(doc, "INTENTIONALLY CROSS-TENANT") {
		t.Error("ListExpiredQuarantineMessages must document why it is intentionally cross-tenant")
	}
}
