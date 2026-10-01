package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// KI-71: the deployment's NATS server (configs/nats/nats-server.conf) requires
// a user; the Go Core connects as "core", the worker as "worker", and the
// worker may not publish what only the Go Core sends. These tests start a
// nats-server with that configuration (NATS_SERVER_BIN or nats-server on
// PATH) and skip without one.

const (
	authTestCorePassword   = "core-test-password"
	authTestWorkerPassword = "worker-test-password"
)

// startAuthServer starts a nats-server with the production configuration and
// returns its address (host:port).
func startAuthServer(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NATS_SERVER_BIN")
	if bin == "" {
		found, err := exec.LookPath("nats-server")
		if err != nil {
			t.Skip("requires a nats-server binary (NATS_SERVER_BIN or nats-server on PATH)")
		}
		bin = found
	}

	conf, err := os.ReadFile(filepath.Join("..", "..", "..", "configs", "nats", "nats-server.conf"))
	if err != nil {
		t.Fatalf("read configs/nats/nats-server.conf: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nats-server.conf"), conf, 0o600); err != nil { //nolint:gosec // G703: the repository's config into the test's temp dir
		t.Fatal(err)
	}
	passwords := fmt.Sprintf("CORE_PASSWORD: %q\nWORKER_PASSWORD: %q\n", authTestCorePassword, authTestWorkerPassword)
	if err := os.WriteFile(filepath.Join(dir, "passwords.conf"), []byte(passwords), 0o600); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cmd := exec.Command(bin, "-c", filepath.Join(dir, "nats-server.conf"), //nolint:gosec // G204: the test's own server binary
		"-a", "127.0.0.1", "-p", fmt.Sprint(port), "-js", "-sd", dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start nats-server: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return addr
		}
		if time.Now().After(deadline) {
			t.Fatalf("nats-server did not start: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// workerConn connects like the worker and records the server's async errors.
func workerConn(t *testing.T, addr string) (nc *nats.Conn, asyncErrors func() []error) {
	t.Helper()
	var mu sync.Mutex
	var asyncErrs []error
	nc, err := nats.Connect("nats://worker:"+authTestWorkerPassword+"@"+addr, nats.CustomInboxPrefix("_INBOX_worker"),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			mu.Lock()
			asyncErrs = append(asyncErrs, err)
			mu.Unlock()
		}))
	if err != nil {
		t.Fatalf("worker connect: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc, func() []error {
		mu.Lock()
		defer mu.Unlock()
		return append([]error(nil), asyncErrs...)
	}
}

func TestAuth_UnauthenticatedConnectionsAreRefused(t *testing.T) {
	addr := startAuthServer(t)
	for name, url := range map[string]string{
		"no user":        "nats://" + addr,
		"wrong password": "nats://worker:wrong@" + addr,
		"unknown user":   "nats://tool:" + authTestWorkerPassword + "@" + addr,
	} {
		t.Run(name, func(t *testing.T) {
			nc, err := nats.Connect(url, nats.NoReconnect())
			if err == nil {
				nc.Close()
				t.Fatal("connection accepted")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "authorization") {
				t.Fatalf("err = %v, want an authorization violation", err)
			}
		})
	}
}

func TestAuth_CoreAndWorkerWorkWithTheirUsers(t *testing.T) {
	addr := startAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The Go Core: stream, KV bucket, a durable subscription, a publish.
	q, err := Connect(ctx, "nats://core:"+authTestCorePassword+"@"+addr, 1<<30)
	if err != nil {
		t.Fatalf("core connect: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	if _, err := q.KeyValue(ctx, "IDEMPOTENCY", time.Hour); err != nil {
		t.Fatalf("core kv: %v", err)
	}
	got := make(chan string, 1)
	stop, err := q.Subscribe(ctx, "agents.output", func(_ context.Context, _ string, data []byte) error {
		got <- string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("core subscribe: %v", err)
	}
	t.Cleanup(stop)

	// The worker: its durable on a Go Core subject, fetch and ack, a result back.
	nc, asyncErrs := workerConn(t, addr)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	cons, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable: "codeforge-py-runs-start", FilterSubject: "runs.start", AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("worker consumer: %v", err)
	}
	if err := q.Publish(ctx, "runs.start", []byte(`{"run_id":"r1"}`)); err != nil {
		t.Fatalf("core publish: %v", err)
	}
	msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("worker fetch: %v", err)
	}
	for msg := range msgs.Messages() {
		if err := msg.DoubleAck(ctx); err != nil {
			t.Fatalf("worker ack: %v", err)
		}
	}
	if _, err := js.Publish(ctx, "agents.output", []byte(`{"task_id":"t1","line":"hello"}`)); err != nil {
		t.Fatalf("worker publish: %v", err)
	}
	select {
	case data := <-got:
		if !strings.Contains(data, "hello") {
			t.Fatalf("core got %q", data)
		}
	case <-ctx.Done():
		t.Fatal("the core did not receive the worker's output")
	}
	if errs := asyncErrs(); len(errs) > 0 {
		t.Fatalf("worker got permission errors: %v", errs)
	}
}

func TestAuth_WorkerCannotForgeCoreMessages(t *testing.T) {
	addr := startAuthServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q, err := Connect(ctx, "nats://core:"+authTestCorePassword+"@"+addr, 1<<30)
	if err != nil {
		t.Fatalf("core connect: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })

	nc, asyncErrs := workerConn(t, addr)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"runs.toolcall.response", "runs.cancel", "runs.start", "conversation.run.start", "conversation.run.cancel", "tasks.agent.aider", "tasks.cancel"} {
		t.Run(subject, func(t *testing.T) {
			pubCtx, pubCancel := context.WithTimeout(ctx, time.Second)
			defer pubCancel()
			if _, err := js.Publish(pubCtx, subject, []byte(`{"run_id":"r1","decision":"allow"}`)); err == nil {
				t.Fatal("the worker published a Go Core subject")
			}
			if !hasPermissionViolation(asyncErrs(), subject) {
				t.Fatalf("no permissions violation for %s: %v", subject, asyncErrs())
			}
		})
	}

	t.Run("core inboxes", func(t *testing.T) {
		// The Go Core's replies and deliveries go to _INBOX_core.*; the worker
		// cannot subscribe there, so it cannot learn their names.
		sub, err := nc.SubscribeSync(inboxPrefix + ".>")
		if err != nil {
			t.Fatal(err)
		}
		if err := q.nc.Publish(inboxPrefix+".x", []byte("reply")); err != nil {
			t.Fatal(err)
		}
		if _, err := sub.NextMsg(500 * time.Millisecond); err == nil {
			t.Fatal("the worker received a message on a Go Core inbox")
		}
		if !hasPermissionViolation(asyncErrs(), inboxPrefix) {
			t.Fatalf("no permissions violation for %s: %v", inboxPrefix, asyncErrs())
		}
	})

	t.Run("stream update", func(t *testing.T) {
		apiCtx, apiCancel := context.WithTimeout(ctx, time.Second)
		defer apiCancel()
		cfg := streamConfig(1 << 30)
		cfg.Subjects = append(cfg.Subjects, "forged.>")
		if _, err := js.UpdateStream(apiCtx, cfg); err == nil {
			t.Fatal("the worker changed the stream")
		}
	})
}

func hasPermissionViolation(errs []error, subject string) bool {
	for _, err := range errs {
		if errors.Is(err, nats.ErrPermissionViolation) && strings.Contains(err.Error(), subject) {
			return true
		}
	}
	return false
}

// TestConnectOpts_CoreInboxes: the Go Core's replies and deliveries go to
// inboxes only its NATS user may subscribe to (KI-71).
func TestConnectOpts_CoreInboxes(t *testing.T) {
	nopts := nats.GetDefaultOptions()
	for _, o := range connectOpts() {
		if err := o(&nopts); err != nil {
			t.Fatalf("applying option: %v", err)
		}
	}
	if nopts.InboxPrefix != "_INBOX_core" {
		t.Fatalf("InboxPrefix = %q, want _INBOX_core", nopts.InboxPrefix)
	}
	if nopts.MaxReconnect != 60 {
		t.Fatalf("connectOpts drops the reconnect options: MaxReconnect = %d", nopts.MaxReconnect)
	}
}
