package svn

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/netutil"
)

// KI-87: the SVN password never appears on svn's command line, which every
// process of the container can read in /proc/<pid>/cmdline (agent tools
// included). svn reads it from stdin (--password-from-stdin, svn 1.10+).

const testPassword = "s3cr3t-Pa55"

// argvLeaks reports the arguments that carry the password.
func argvLeaks(args []string) []string {
	var leaks []string
	for _, a := range args {
		if strings.Contains(a, testPassword) || a == "--password" || strings.HasPrefix(a, "--password=") {
			leaks = append(leaks, a)
		}
	}
	return leaks
}

func TestSVN_PasswordNeverInArgv(t *testing.T) {
	ctx := context.Background()
	operations := []struct {
		name string
		run  func(p *Provider, wc string) error
	}{
		{"clone", func(p *Provider, _ string) error {
			return p.Clone(ctx, projectURL, filepath.Join(t.TempDir(), "wc"))
		}},
		{"reclone", func(p *Provider, wc string) error { return p.Clone(ctx, projectURL, wc) }},
		{"pull", func(p *Provider, wc string) error { return p.Pull(ctx, wc) }},
		{"status", func(p *Provider, wc string) error { _, err := p.Status(ctx, wc); return err }},
		{"list branches", func(p *Provider, wc string) error { _, err := p.ListBranches(ctx, wc); return err }},
		{"checkout", func(p *Provider, wc string) error { return p.Checkout(ctx, wc, "feature") }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			p, fake := newCredentialedProvider(projectURL, "https://svn.example.com/repos/proj")
			p.password = testPassword
			if err := op.run(p, newWorkingCopy(t)); err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			if len(fake.calls) == 0 {
				t.Fatal("svn was not run")
			}
			for _, args := range fake.calls {
				if leaks := argvLeaks(args); len(leaks) > 0 {
					t.Fatalf("svn %s argv carries the password: %v (argv %v)", subcommand(args), leaks, args)
				}
				if !slices.Contains(args, "--password-from-stdin") || !slices.Contains(args, "--username") {
					t.Fatalf("svn %s argv %v: want --username and --password-from-stdin", subcommand(args), args)
				}
			}
		})
	}
}

func TestSVN_PasswordGoesToStdin(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		password  string
		wantStdin string
		wantFlag  bool
	}{
		{name: "username and password", username: "alice", password: testPassword, wantStdin: testPassword, wantFlag: true},
		{name: "password with spaces and quotes", username: "alice", password: ` a"b'c\ `, wantStdin: ` a"b'c\ `, wantFlag: true},
		{name: "non-ASCII password", username: "alice", password: "pässwörd", wantStdin: "pässwörd", wantFlag: true},
		{name: "username only", username: "alice"},
		{name: "no credentials"},
		{name: "password without username is not sent", password: testPassword},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var argv []string
			p := NewProvider(nil)
			p.username, p.password = tc.username, tc.password
			// cat echoes what svn would read from stdin.
			p.execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				argv = args
				return exec.CommandContext(ctx, "cat")
			}
			stdin, err := p.runSVN(context.Background(), "", "info")
			if err != nil {
				t.Fatalf("runSVN: %v", err)
			}
			if stdin != tc.wantStdin {
				t.Fatalf("stdin = %q, want %q", stdin, tc.wantStdin)
			}
			if got := slices.Contains(argv, "--password-from-stdin"); got != tc.wantFlag {
				t.Fatalf("--password-from-stdin in argv %v = %v, want %v", argv, got, tc.wantFlag)
			}
			if leaks := argvLeaks(argv); len(leaks) > 0 {
				t.Fatalf("argv carries the password: %v", leaks)
			}
		})
	}
}

// svn reads one line from stdin: a password with a line break would be cut
// and sent truncated, so it is refused before svn runs.
func TestSVN_PasswordWithLineBreakIsRefused(t *testing.T) {
	for _, pw := range []string{"abc\ndef", "abc\r\n", "\n"} {
		p, fake := newFakeProvider("https://svn.example.com/repo")
		p.username, p.password = "alice", pw
		if _, err := p.runSVN(context.Background(), "", "info"); err == nil {
			t.Errorf("password %q: runSVN succeeded", pw)
		}
		if len(fake.calls) != 0 {
			t.Errorf("password %q: svn ran: %v", pw, fake.calls)
		}
	}
}

// TestSVN_PasswordFromStdinWithRealServer checks the flags against a real
// svn client and svnserve with password authentication (skipped without
// them): the checkout authenticates, a wrong password fails.
func TestSVN_PasswordFromStdinWithRealServer(t *testing.T) {
	for _, bin := range []string{"svn", "svnadmin", "svnserve"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("requires %s", bin)
		}
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if out, err := exec.Command("svnadmin", "create", repo).CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v: %s", err, out)
	}
	conf := "[general]\nanon-access = none\nauth-access = write\npassword-db = passwd\n"
	if err := os.WriteFile(filepath.Join(repo, "conf", "svnserve.conf"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "conf", "passwd"), []byte("[users]\nalice = "+testPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	server := exec.Command("svnserve", "-d", "--foreground", "-r", root, //nolint:gosec // G204: the test's own server
		"--listen-host", "127.0.0.1", "--listen-port", fmt.Sprint(port))
	if err := server.Start(); err != nil {
		t.Fatalf("start svnserve: %v", err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for deadline := time.Now().Add(10 * time.Second); ; {
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("svnserve did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}

	url := "svn://" + addr + "/repo"
	loopback, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"}) // as svn.allowed_private_hosts would
	if err != nil {
		t.Fatal(err)
	}
	newProvider := func(password string) *Provider {
		p := NewProvider(nil)
		p.username, p.password, p.repoURL = "alice", password, url
		p.outbound = loopback
		return p
	}
	ctx := context.Background()
	if err := newProvider(testPassword).Clone(ctx, url, filepath.Join(root, "wc")); err != nil {
		t.Fatalf("checkout with the password from stdin: %v", err)
	}
	if err := newProvider("wrong").Clone(ctx, url, filepath.Join(root, "wc-wrong")); err == nil {
		t.Fatal("checkout with a wrong password succeeded")
	}
}
