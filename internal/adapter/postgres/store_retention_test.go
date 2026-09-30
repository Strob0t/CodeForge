package postgres_test

import (
	"context"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
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

// purgeFunc is the signature of every retention store method.
type purgeFunc func(ctx context.Context, before time.Time, batchSize int) (int64, error)

// purgeAll calls a retention method in batches of one row (so several
// batches run) until a batch comes back short, like RetentionService does.
// The system job runs without a tenant in the context.
func purgeAll(t *testing.T, name string, purge purgeFunc) {
	t.Helper()
	const batch = 1
	for range 10_000 {
		n, err := purge(context.Background(), retentionCutoff, batch)
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
		"store_retention.go": {"DeleteExpiredSessions", "DeleteExpiredConversations", "DeleteExpiredRuns", "DeleteExpiredAuditEntries"},
		"store_audit_log.go": {"AnonymizeExpiredIPAddresses"},
		"store_consent.go":   {"AnonymizeExpiredConsentIPAddresses"},
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

	purgeAll(t, "DeleteExpiredSessions", a.store.DeleteExpiredSessions)

	assertRows(t, pool, "sessions",
		map[string]string{"idle inside retention": idleA.ID, "recently used": activeA.ID, "other tenant, fresh": freshB.ID},
		map[string]string{"expired": oldA.ID, "other tenant, expired": oldB.ID})
}

func TestStore_DeleteExpiredConversations(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)

	oldA := a.conversation(t)
	msg, err := a.store.CreateMessage(a.ctx, &conversation.Message{ConversationID: oldA.ID, Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
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
	backdateConversation(t, pool, oldA.ID, expired)
	backdateConversation(t, pool, oldB.ID, expired)
	backdateConversation(t, pool, idleA.ID, inside)
	backdateConversation(t, pool, activeA.ID, fresh) // a message was added recently

	purgeAll(t, "DeleteExpiredConversations", a.store.DeleteExpiredConversations)

	assertRows(t, pool, "conversations",
		map[string]string{"idle inside retention": idleA.ID, "recently used": activeA.ID, "other tenant, fresh": freshB.ID},
		map[string]string{"expired": oldA.ID, "other tenant, expired": oldB.ID})
	assertRows(t, pool, "conversation_messages", nil, map[string]string{"message of expired conversation": msg.ID})
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

	purgeAll(t, "DeleteExpiredSessions", f.store.DeleteExpiredSessions)
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

	purgeAll(t, "DeleteExpiredRuns", f.store.DeleteExpiredRuns) // sets the session's current_run_id to NULL
	purgeAll(t, "DeleteExpiredSessions", f.store.DeleteExpiredSessions)

	assertRows(t, pool, "sessions", nil, map[string]string{"expired session of an expired run": sess.ID})
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

	purgeAll(t, "DeleteExpiredRuns", a.store.DeleteExpiredRuns)

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

	purgeAll(t, "DeleteExpiredAuditEntries", a.store.DeleteExpiredAuditEntries)

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

	purgeAll(t, "AnonymizeExpiredIPAddresses", a.store.AnonymizeExpiredIPAddresses)

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

	purgeAll(t, "AnonymizeExpiredConsentIPAddresses", a.store.AnonymizeExpiredConsentIPAddresses)

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
