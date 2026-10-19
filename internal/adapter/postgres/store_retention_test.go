package postgres_test

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Data retention (KI-52) is one instance-wide policy, so each purge query
// spans all tenants and removes (or anonymizes) only rows whose age is past
// the cutoff, at most batchSize per call. The test rows are dated centuries
// back and the cutoff lies 200 years back, so rows other tests keep in the
// shared database are never touched.

const (
	expired = "300 years" // past the test cutoff
	inside  = "100 years" // still inside the test retention period
	fresh   = "0 seconds"
)

var retentionCutoff = time.Now().AddDate(-200, 0, 0)

// purgeFunc is a retention purge with one batch size.
type purgeFunc func(ctx context.Context, before time.Time, batchSize int) (int64, error)

// purgeMethod picks a purge of the purger.
type purgeMethod func(p database.RetentionPurger) purgeFunc

var (
	purgeSessions purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.DeleteExpiredSessions }
	// purgeConversations deletes one message per statement, so a conversation
	// with several messages needs several.
	purgeConversations purgeMethod = func(p database.RetentionPurger) purgeFunc {
		return func(ctx context.Context, before time.Time, batchSize int) (int64, error) {
			return p.DeleteExpiredConversations(ctx, before, batchSize, 1)
		}
	}
	purgeRuns         purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.DeleteExpiredRuns }
	purgeAuditEntries purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.DeleteExpiredAuditEntries }
	purgeAuditIPs     purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.AnonymizeExpiredIPAddresses }
	purgeConsentIPs   purgeMethod = func(p database.RetentionPurger) purgeFunc { return p.AnonymizeExpiredConsentIPAddresses }
)

// withPurger runs fn under the retention lock, like a sweep. The system job
// runs without a tenant in the context.
func withPurger(ctx context.Context, t *testing.T, store *postgres.Store, fn func(ctx context.Context, p database.RetentionPurger)) {
	t.Helper()
	acquired, err := store.WithRetentionLock(ctx, fn)
	if err != nil || !acquired {
		t.Fatalf("retention lock: acquired %v, err %v", acquired, err)
	}
}

// purgeAll calls a retention purge in batches of one row (so several batches
// run) until a batch comes back short, like RetentionService does.
func purgeAll(t *testing.T, store *postgres.Store, name string, method purgeMethod) {
	t.Helper()
	const batch = 1
	withPurger(context.Background(), t, store, func(ctx context.Context, p database.RetentionPurger) {
		purge := method(p)
		for range 10_000 {
			n, err := purge(ctx, retentionCutoff, batch)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n > batch {
				t.Fatalf("%s changed %d rows in one call, batch size is %d", name, n, batch)
			}
			if n < batch {
				return
			}
		}
		t.Fatalf("%s did not finish", name)
	})
}

func retentionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// backdated fails the test unless a backdate statement changed exactly one row.
func backdated(t *testing.T, tag pgconn.CommandTag, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("backdate touched %d rows, want 1", tag.RowsAffected())
	}
}

// The backdate helpers date a row's creation past the cutoff and its last
// activity lastActiveAgo back: retention counts from the last activity, so
// only rows idle past the cutoff may go. Plain updates: no timestamp retention
// reads is set by a trigger.

func backdateSession(t *testing.T, pool *pgxpool.Pool, id, lastActiveAgo string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(),
		`UPDATE sessions SET created_at = now() - $2::interval, last_activity_at = now() - $3::interval WHERE id = $1`,
		id, expired, lastActiveAgo)
	backdated(t, tag, err)
}

func backdateConversation(t *testing.T, pool *pgxpool.Pool, id, lastActiveAgo string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(),
		`UPDATE conversations SET created_at = now() - $2::interval, updated_at = now() - $3::interval WHERE id = $1`,
		id, expired, lastActiveAgo)
	backdated(t, tag, err)
}

func backdateRun(t *testing.T, pool *pgxpool.Pool, id, lastActiveAgo string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(),
		`UPDATE runs SET created_at = now() - $2::interval, updated_at = now() - $3::interval WHERE id = $1`,
		id, expired, lastActiveAgo)
	backdated(t, tag, err)
}

func backdateAuditEntry(t *testing.T, pool *pgxpool.Pool, id, createdAgo string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `UPDATE audit_log SET created_at = now() - $2::interval WHERE id = $1`, id, createdAgo)
	backdated(t, tag, err)
}

var existsSQL = map[string]string{
	"sessions":              `SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1)`,
	"conversations":         `SELECT EXISTS (SELECT 1 FROM conversations WHERE id = $1)`,
	"conversation_messages": `SELECT EXISTS (SELECT 1 FROM conversation_messages WHERE id = $1)`,
	"runs":                  `SELECT EXISTS (SELECT 1 FROM runs WHERE id = $1)`,
	"audit_log":             `SELECT EXISTS (SELECT 1 FROM audit_log WHERE id = $1)`,
}

func rowExists(t *testing.T, pool *pgxpool.Pool, table, id string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), existsSQL[table], id).Scan(&exists); err != nil {
		t.Fatalf("exists %s %s: %v", table, id, err)
	}
	return exists
}

// assertRows checks which rows of a table the purge kept and removed.
func assertRows(t *testing.T, pool *pgxpool.Pool, table string, kept, removed map[string]string) {
	t.Helper()
	for name, id := range kept {
		if !rowExists(t, pool, table, id) {
			t.Errorf("%s: %s (%s) was removed, want it kept", table, name, id)
		}
	}
	for name, id := range removed {
		if rowExists(t, pool, table, id) {
			t.Errorf("%s: %s (%s) was kept, want it removed", table, name, id)
		}
	}
}

func (f *statusFixture) taskSession(t *testing.T) *run.Session {
	t.Helper()
	sess := &run.Session{ProjectID: f.project.ID, TaskID: f.task.ID, Status: run.SessionStatusActive, Metadata: "{}"}
	if err := f.store.CreateSession(f.ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sess
}

func (f *statusFixture) conversation(t *testing.T) *conversation.Conversation {
	t.Helper()
	c, err := f.store.CreateConversation(f.ctx, &conversation.Conversation{ProjectID: f.project.ID, Title: "retention"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	return c
}

func TestRetentionQueries_IntentionallyCrossTenant(t *testing.T) {
	methods := map[string][]string{
		"store_retention.go": {
			"WithRetentionLock", "DeleteExpiredSessions", "DeleteExpiredConversations", "DeleteExpiredRuns",
			"DeleteExpiredAuditEntries", "AnonymizeExpiredIPAddresses", "AnonymizeExpiredConsentIPAddresses",
		},
	}
	for file, names := range methods {
		src := readStoreSource(t, file)
		for _, name := range names {
			if doc := methodDocComment(t, src, file, name); !strings.Contains(doc, "INTENTIONALLY CROSS-TENANT") {
				t.Errorf("%s must document why it is intentionally cross-tenant", name)
			}
		}
	}
}

func TestStore_DeleteExpiredSessions(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA, oldB := a.taskSession(t), b.taskSession(t)
	idleA := a.taskSession(t)
	activeA := a.taskSession(t) // created long ago, used recently
	freshB := b.taskSession(t)
	backdateSession(t, pool, oldA.ID, expired)
	backdateSession(t, pool, oldB.ID, expired)
	backdateSession(t, pool, idleA.ID, inside)
	backdateSession(t, pool, activeA.ID, fresh)

	purgeAll(t, a.store, "DeleteExpiredSessions", purgeSessions)

	assertRows(t, pool, "sessions",
		map[string]string{"idle inside retention": idleA.ID, "recently used": activeA.ID, "other tenant, fresh": freshB.ID},
		map[string]string{"expired": oldA.ID, "other tenant, expired": oldB.ID})
}

func TestStore_DeleteExpiredConversations(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA := a.conversation(t)
	oldMsgs := map[string]string{"first": a.message(t, oldA.ID), "second": a.message(t, oldA.ID), "third": a.message(t, oldA.ID)}
	// Every agentic conversation has a session (EnsureConversationSession).
	// It has no task, so the FK's SET NULL would violate the sessions
	// check constraint: the purge removes it with its conversation.
	convSession := &run.Session{ProjectID: a.project.ID, ConversationID: oldA.ID, Status: run.SessionStatusActive, Metadata: "{}"}
	if err := a.store.CreateSession(a.ctx, convSession); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// A session that also belongs to a task is kept and only detached.
	taskSession := &run.Session{ProjectID: a.project.ID, TaskID: a.task.ID, ConversationID: oldA.ID, Status: run.SessionStatusActive, Metadata: "{}"}
	if err := a.store.CreateSession(a.ctx, taskSession); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	idleA, activeA := a.conversation(t), a.conversation(t)
	oldB, freshB := b.conversation(t), b.conversation(t)
	oldMsgs["other tenant"] = b.message(t, oldB.ID)
	idleMsg := a.message(t, idleA.ID)
	backdateConversation(t, pool, oldA.ID, expired)
	backdateConversation(t, pool, oldB.ID, expired)
	backdateConversation(t, pool, idleA.ID, inside)
	backdateConversation(t, pool, activeA.ID, fresh) // a message was added recently

	purgeAll(t, a.store, "DeleteExpiredConversations", purgeConversations)

	assertRows(t, pool, "conversations",
		map[string]string{"idle inside retention": idleA.ID, "recently used": activeA.ID, "other tenant, fresh": freshB.ID},
		map[string]string{"expired": oldA.ID, "other tenant, expired": oldB.ID})
	assertRows(t, pool, "conversation_messages", map[string]string{"inside retention": idleMsg}, oldMsgs)
	assertRows(t, pool, "sessions",
		map[string]string{"task session": taskSession.ID},
		map[string]string{"conversation session": convSession.ID})
	kept, err := a.store.GetSession(a.ctx, taskSession.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if kept.ConversationID != "" || kept.TaskID != a.task.ID {
		t.Fatalf("task session = task %q conversation %q, want task %q and no conversation", kept.TaskID, kept.ConversationID, a.task.ID)
	}
}

func (f *statusFixture) message(t *testing.T, convID string) string {
	t.Helper()
	m, err := f.store.CreateMessage(f.ctx, &conversation.Message{ConversationID: convID, Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return m.ID
}

// waitForLockWait returns once a backend runs a statement starting with
// prefix ("" for any) and waits for a lock.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND starts_with(ltrim(query), $1)`,
			prefix).Scan(&waiting); err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no statement %q waited for a lock", prefix)
}

// CreateMessage records the conversation's activity in the statement that
// adds the message: a purge that still sees the conversation as idle cannot
// find a message added to it. The test holds a share lock on the
// conversation, which blocks the activity update but not a foreign key
// check: while CreateMessage waits, none of its message is visible.
func TestCreateMessage_ActivityAndMessageAreOneWrite(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	conv := f.conversation(t)
	backdateConversation(t, pool, conv.ID, expired)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM conversations WHERE id = $1 FOR SHARE`, conv.ID); err != nil {
		t.Fatalf("share lock: %v", err)
	}

	created := make(chan error, 1)
	go func() {
		_, err := f.store.CreateMessage(f.ctx, &conversation.Message{ConversationID: conv.ID, Role: "user", Content: "late"})
		created <- err
	}()
	waitForLockWait(t, pool, "")
	var visible int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM conversation_messages WHERE conversation_id = $1`, conv.ID).Scan(&visible); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if visible != 0 {
		t.Fatalf("%d message(s) added while the conversation still looked idle", visible)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-created; err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	var active bool
	if err := pool.QueryRow(ctx, `SELECT updated_at > now() - interval '1 minute' FROM conversations WHERE id = $1`, conv.ID).Scan(&active); err != nil || !active {
		t.Fatalf("conversation activity not recorded: active %v, err %v", active, err)
	}
}

// A message for a conversation that a purge batch holds and deletes is not
// found (it used to fail on the foreign key after waiting for the purge).
func TestCreateMessage_ConversationPurgedMeanwhile(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	conv := f.conversation(t)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM conversations WHERE id = $1 FOR UPDATE`, conv.ID); err != nil {
		t.Fatalf("lock like a purge batch: %v", err)
	}

	created := make(chan error, 1)
	go func() {
		_, err := f.store.CreateMessage(f.ctx, &conversation.Message{ConversationID: conv.ID, Role: "user", Content: "late"})
		created <- err
	}()
	waitForLockWait(t, pool, "")
	if _, err := tx.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, conv.ID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-created; !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("CreateMessage = %v, want not found", err)
	}
}

// A conversation being written while the purge runs is passed over, not
// waited for, and keeps its messages: the purge locks its batch with
// FOR UPDATE SKIP LOCKED (and PostgreSQL re-checks the age of a conversation
// written after the purge's snapshot).
func TestRetention_ConversationBeingWrittenIsKept(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	conv := f.conversation(t)
	msg := f.message(t, conv.ID)
	backdateConversation(t, pool, conv.ID, expired)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conv.ID); err != nil {
		t.Fatalf("reactivate: %v", err)
	}

	withPurger(ctx, t, f.store, func(ctx context.Context, p database.RetentionPurger) {
		if _, err := p.DeleteExpiredConversations(ctx, retentionCutoff, 1000, 1000); err != nil {
			t.Fatalf("DeleteExpiredConversations waited for the writer or failed: %v", err)
		}
	})
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertRows(t, pool, "conversations", map[string]string{"written during the purge": conv.ID}, nil)
	assertRows(t, pool, "conversation_messages", map[string]string{"its message": msg}, nil)
}

// A sweep runs its statements on the connection that holds the retention
// lock, so it works with a pool of a single connection (its statements used
// to wait for a second one while the lock connection sat idle).
func TestRetention_SweepNeedsOneConnection(t *testing.T) {
	setupStore(t) // runs the migrations
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := postgres.NewStore(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	purges := map[string]purgeMethod{
		"sessions": purgeSessions, "conversations": purgeConversations, "runs": purgeRuns,
		"audit entries": purgeAuditEntries, "audit IPs": purgeAuditIPs, "consent IPs": purgeConsentIPs,
	}
	withPurger(ctx, t, store, func(ctx context.Context, p database.RetentionPurger) {
		for name, method := range purges {
			if _, err := method(p)(ctx, retentionCutoff, 1000); err != nil {
				t.Errorf("%s with a pool of one connection: %v", name, err)
			}
		}
	})
}

// A conversation's session is reused by every message (no other write), so
// reusing it must count as activity: the session of a conversation in daily
// use survives however old it is.
func TestRetention_InUseConversationSessionSurvives(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	conv := f.conversation(t)
	sessions := service.NewSessionService(f.store, nil)

	sess, err := sessions.EnsureConversationSession(f.ctx, f.project.ID, conv.ID)
	if err != nil {
		t.Fatalf("EnsureConversationSession: %v", err)
	}
	backdateSession(t, pool, sess.ID, expired)

	reused, err := sessions.EnsureConversationSession(f.ctx, f.project.ID, conv.ID) // the next message
	if err != nil {
		t.Fatalf("EnsureConversationSession: %v", err)
	}
	if reused.ID != sess.ID {
		t.Fatalf("reused session %s, want %s", reused.ID, sess.ID)
	}

	purgeAll(t, f.store, "DeleteExpiredSessions", purgeSessions)
	assertRows(t, pool, "sessions", map[string]string{"session in use": sess.ID}, nil)
}

// Foreign key actions (SET NULL when a referenced run or conversation is
// purged) are not activity: an expired session referencing an expired run
// still goes.
func TestRetention_ForeignKeyNullingIsNotSessionActivity(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	r := f.newRun(t, run.StatusCompleted)
	sess := &run.Session{ProjectID: f.project.ID, TaskID: f.task.ID, CurrentRunID: r.ID, Status: run.SessionStatusActive}
	if err := f.store.CreateSession(f.ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	backdateSession(t, pool, sess.ID, expired)
	backdateRun(t, pool, r.ID, expired)

	purgeAll(t, f.store, "DeleteExpiredRuns", purgeRuns) // sets the session's current_run_id to NULL
	purgeAll(t, f.store, "DeleteExpiredSessions", purgeSessions)

	assertRows(t, pool, "sessions", nil, map[string]string{"expired session of an expired run": sess.ID})
}

// The cost-record retention deletes runs, not the plan steps that ran them:
// a plan step keeps its status and error, only its run reference goes.
func TestRetention_RunPurgeKeepsPlanSteps(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	p := &plan.ExecutionPlan{
		ProjectID: f.project.ID, Name: "retention", Protocol: plan.ProtocolSequential, Status: plan.StatusCompleted, MaxParallel: 1,
		Steps: []plan.Step{{TaskID: f.task.ID, AgentID: f.agent.ID, Status: plan.StepStatusPending}},
	}
	if err := f.store.CreatePlan(f.ctx, p); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	r := f.newRun(t, run.StatusFailed)
	if err := f.store.UpdatePlanStepStatus(f.ctx, p.Steps[0].ID, plan.StepStatusFailed, r.ID, "gate failed"); err != nil {
		t.Fatalf("UpdatePlanStepStatus: %v", err)
	}
	backdateRun(t, pool, r.ID, expired)

	purgeAll(t, f.store, "DeleteExpiredRuns", purgeRuns)

	steps, err := f.store.ListPlanSteps(f.ctx, p.ID)
	if err != nil {
		t.Fatalf("ListPlanSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("plan has %d steps after the run purge, want 1", len(steps))
	}
	if st := steps[0]; st.RunID != "" || st.Status != plan.StepStatusFailed || st.Error != "gate failed" {
		t.Fatalf("step = run %q status %q error %q, want no run, failed, gate failed", st.RunID, st.Status, st.Error)
	}
}

func TestStore_DeleteExpiredRuns(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA, oldB := a.newRun(t, run.StatusCompleted), b.newRun(t, run.StatusFailed)
	idleA := a.newRun(t, run.StatusCompleted)
	activeA := a.newRun(t, run.StatusRunning) // started long ago, updated recently
	freshB := b.newRun(t, run.StatusCompleted)
	backdateRun(t, pool, oldA.ID, expired)
	backdateRun(t, pool, oldB.ID, expired)
	backdateRun(t, pool, idleA.ID, inside)
	backdateRun(t, pool, activeA.ID, fresh)

	purgeAll(t, a.store, "DeleteExpiredRuns", purgeRuns)

	assertRows(t, pool, "runs",
		map[string]string{"idle inside retention": idleA.ID, "recently updated": activeA.ID, "other tenant, fresh": freshB.ID},
		map[string]string{"expired": oldA.ID, "other tenant, expired": oldB.ID})
}

// auditEntry inserts an audit entry with an IP address and returns its ID.
func (f *statusFixture) auditEntry(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	email := "retention-" + uuid.New().String()[:8] + "@example.com"
	marker := uuid.New().String()
	if err := f.store.InsertAuditEntry(f.ctx, &database.AuditEntry{
		AdminID: uuid.New().String(), AdminEmail: &email, Action: "user.update",
		Resource: "user", ResourceID: marker, IPAddress: "203.0.113.7",
	}); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM audit_log WHERE resource_id = $1`, marker).Scan(&id); err != nil {
		t.Fatalf("find audit entry: %v", err)
	}
	return id
}

func TestStore_DeleteExpiredAuditEntries(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA, oldB := a.auditEntry(t, pool), b.auditEntry(t, pool)
	idleA, freshB := a.auditEntry(t, pool), b.auditEntry(t, pool)
	backdateAuditEntry(t, pool, oldA, expired)
	backdateAuditEntry(t, pool, oldB, expired)
	backdateAuditEntry(t, pool, idleA, inside)

	purgeAll(t, a.store, "DeleteExpiredAuditEntries", purgeAuditEntries)

	assertRows(t, pool, "audit_log",
		map[string]string{"inside retention": idleA, "other tenant, fresh": freshB},
		map[string]string{"expired": oldA, "other tenant, expired": oldB})
}

func TestStore_AnonymizeExpiredIPAddresses(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA, oldB := a.auditEntry(t, pool), b.auditEntry(t, pool)
	idleA, freshB := a.auditEntry(t, pool), b.auditEntry(t, pool)
	backdateAuditEntry(t, pool, oldA, expired)
	backdateAuditEntry(t, pool, oldB, expired)
	backdateAuditEntry(t, pool, idleA, inside)

	purgeAll(t, a.store, "AnonymizeExpiredIPAddresses", purgeAuditIPs)

	for name, tc := range map[string]struct {
		id     string
		wantIP bool
	}{
		"expired":               {oldA, false},
		"other tenant, expired": {oldB, false},
		"inside retention":      {idleA, true},
		"other tenant, fresh":   {freshB, true},
	} {
		var ip *netip.Addr
		var email *string
		if err := pool.QueryRow(context.Background(),
			`SELECT ip_address, admin_email FROM audit_log WHERE id = $1`, tc.id).Scan(&ip, &email); err != nil {
			t.Fatalf("%s: entry must be kept: %v", name, err)
		}
		if (ip != nil) != tc.wantIP {
			t.Errorf("%s: ip_address = %v, want kept %v", name, ip, tc.wantIP)
		}
		if email == nil {
			t.Errorf("%s: admin_email was removed, only the IP address expires", name)
		}
	}
}

// consentRecord records a consent with IP address and user agent for a new
// user of the fixture's tenant and returns the record's ID.
func (f *statusFixture) consentRecord(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	u := createAuditTestUser(t, f.store, middleware.TenantIDFromContext(f.ctx))
	recordConsent(f.ctx, t, f.store, u.ID)
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM user_consents WHERE user_id = $1`, u.ID).Scan(&id); err != nil {
		t.Fatalf("find consent: %v", err)
	}
	return id
}

func backdateConsent(t *testing.T, pool *pgxpool.Pool, id, createdAgo string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `UPDATE user_consents SET created_at = now() - $2::interval WHERE id = $1`, id, createdAgo)
	backdated(t, tag, err)
}

func TestStore_AnonymizeExpiredConsentIPAddresses(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA, oldB := a.consentRecord(t, pool), b.consentRecord(t, pool)
	idleA, freshB := a.consentRecord(t, pool), b.consentRecord(t, pool)
	backdateConsent(t, pool, oldA, expired)
	backdateConsent(t, pool, oldB, expired)
	backdateConsent(t, pool, idleA, inside)

	purgeAll(t, a.store, "AnonymizeExpiredConsentIPAddresses", purgeConsentIPs)

	for name, tc := range map[string]struct {
		id         string
		wantClient bool
	}{
		"expired":               {oldA, false},
		"other tenant, expired": {oldB, false},
		"inside retention":      {idleA, true},
		"other tenant, fresh":   {freshB, true},
	} {
		var ip *netip.Addr
		var userAgent, userID *string
		var granted bool
		if err := pool.QueryRow(context.Background(),
			`SELECT ip_address, user_agent, user_id::text, granted FROM user_consents WHERE id = $1`, tc.id).
			Scan(&ip, &userAgent, &userID, &granted); err != nil {
			t.Fatalf("%s: consent record must be kept: %v", name, err)
		}
		if (ip != nil) != tc.wantClient || (userAgent != nil) != tc.wantClient {
			t.Errorf("%s: ip_address = %v, user_agent = %v, want kept %v", name, ip, userAgent, tc.wantClient)
		}
		if userID == nil || !granted {
			t.Errorf("%s: user or decision removed, only IP address and user agent expire", name)
		}
	}
}

// The retention lock lets one sweep run at a time across replicas: while a
// sweep holds it, another store (another replica's connection pool) does not
// get it; afterwards it does.
func TestStore_RetentionLockIsExclusive(t *testing.T) {
	first, second := setupStore(t), setupStore(t)
	ctx := context.Background()

	var innerRan bool
	outer, err := first.WithRetentionLock(ctx, func(ctx context.Context, _ database.RetentionPurger) {
		inner, err := second.WithRetentionLock(ctx, func(context.Context, database.RetentionPurger) { innerRan = true })
		if err != nil || inner {
			t.Errorf("second replica got the lock during a sweep: acquired %v, err %v", inner, err)
		}
	})
	if err != nil || !outer {
		t.Fatalf("first replica: acquired %v, err %v", outer, err)
	}
	if innerRan {
		t.Fatal("the second sweep ran while the first held the lock")
	}

	again, err := second.WithRetentionLock(ctx, func(context.Context, database.RetentionPurger) { innerRan = true })
	if err != nil || !again || !innerRan {
		t.Fatalf("after the first sweep: acquired %v, ran %v, err %v, want the lock released", again, innerRan, err)
	}
}

// The retention batches of audit entries and consent records are selected
// through an index on their creation (with sequential scans disabled, the
// plan names a retention index). The IP address batch can use the partial
// index or the plain one; the planner picks by the table's statistics, so
// either counts. Sessions, conversations and runs age by an activity
// timestamp that every update changes; they are scanned, so that their
// updates stay HOT (migration 096).
func TestRetention_BatchSelectionUsesIndexes(t *testing.T) {
	setupStore(t) // runs the migrations
	pool := retentionPool(t)
	tests := []struct {
		name    string
		query   string
		indexes []string
	}{
		{"audit entries", `SELECT id FROM audit_log WHERE created_at < $1 LIMIT 1000`,
			[]string{"idx_retention_audit_log_created"}},
		{"audit IP addresses", `SELECT id FROM audit_log WHERE ip_address IS NOT NULL AND created_at < $1 LIMIT 1000`,
			[]string{"idx_retention_audit_log_ip_created", "idx_retention_audit_log_created"}},
		{"consent IP addresses", `SELECT id FROM user_consents WHERE (ip_address IS NOT NULL OR user_agent IS NOT NULL) AND created_at < $1 LIMIT 1000`,
			[]string{"idx_retention_user_consents_client_created"}},
	}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
				t.Fatalf("disable seqscan: %v", err)
			}
			rows, err := tx.Query(ctx, `EXPLAIN `+tt.query, retentionCutoff)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			var explained strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatalf("scan plan: %v", err)
				}
				explained.WriteString(line + "\n")
			}
			rows.Close()
			if !slices.ContainsFunc(tt.indexes, func(index string) bool { return strings.Contains(explained.String(), index) }) {
				t.Fatalf("plan uses none of %v:\n%s", tt.indexes, explained.String())
			}
		})
	}
}
