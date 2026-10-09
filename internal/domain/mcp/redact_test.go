package mcp

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// KI-71 review: env variables and headers of an MCP server carry
// credentials. The API shows which are set (keys, RedactedValue), never a
// value; a client that sends RedactedValue back keeps the stored value.

func TestServerDef_Redacted(t *testing.T) {
	stored := ServerDef{
		ID: "s1", Name: "github", Transport: TransportStdio, Command: "mcp-github", Args: []string{"--stdio"},
		Env:     map[string]string{"GITHUB_TOKEN": "ghp_secret", "EMPTY": ""},
		Headers: map[string]string{"Authorization": "Bearer secret"},
	}

	got := stored.Redacted()

	if want := map[string]string{"GITHUB_TOKEN": RedactedValue, "EMPTY": ""}; !maps.Equal(got.Env, want) {
		t.Errorf("Env = %v, want %v", got.Env, want)
	}
	if want := map[string]string{"Authorization": RedactedValue}; !maps.Equal(got.Headers, want) {
		t.Errorf("Headers = %v, want %v", got.Headers, want)
	}
	if got.ID != "s1" || got.Command != "mcp-github" || len(got.Args) != 1 {
		t.Errorf("Redacted changed other fields: %+v", got)
	}
	if stored.Env["GITHUB_TOKEN"] != "ghp_secret" || stored.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("Redacted changed the stored definition: %+v", stored)
	}
	if empty := (&ServerDef{}).Redacted(); empty.Env != nil || empty.Headers != nil {
		t.Errorf("Redacted of a server without env and headers = %+v, want nil maps", empty)
	}
}

func TestServerDef_KeepRedacted(t *testing.T) {
	stored := &ServerDef{
		Env:     map[string]string{"TOKEN": "secret", "REGION": "eu"},
		Headers: map[string]string{"Authorization": "Bearer secret"},
	}
	tests := []struct {
		name        string
		env         map[string]string
		headers     map[string]string
		stored      *ServerDef
		wantEnv     map[string]string
		wantHeaders map[string]string
		wantErr     bool
	}{
		{
			name:        "sent back unchanged",
			env:         map[string]string{"TOKEN": RedactedValue, "REGION": RedactedValue},
			headers:     map[string]string{"Authorization": RedactedValue},
			stored:      stored,
			wantEnv:     map[string]string{"TOKEN": "secret", "REGION": "eu"},
			wantHeaders: map[string]string{"Authorization": "Bearer secret"},
		},
		{
			// S7-G review: a kept header would go to a server whose env was
			// changed around it (TestServerDef_KeepRedactedOnlyWithUnchangedEnvAndHeaders).
			name:    "one value changed, one removed, one added while a header is kept",
			env:     map[string]string{"TOKEN": "new-secret", "NEW": "value"},
			headers: map[string]string{"Authorization": RedactedValue},
			stored:  stored,
			wantErr: true,
		},
		{
			name:    "every secret entered again",
			env:     map[string]string{"TOKEN": "new-secret", "NEW": "value"},
			headers: map[string]string{"Authorization": "Bearer new"},
			stored:  stored,
			wantEnv: map[string]string{"TOKEN": "new-secret", "NEW": "value"}, wantHeaders: map[string]string{"Authorization": "Bearer new"},
		},
		{name: "nothing redacted", env: map[string]string{"A": "b"}, stored: nil, wantEnv: map[string]string{"A": "b"}},
		{name: "redacted value of a new key", env: map[string]string{"OTHER": RedactedValue}, stored: stored, wantErr: true},
		{name: "redacted value without a stored server", headers: map[string]string{"Authorization": RedactedValue}, stored: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &ServerDef{Env: tt.env, Headers: tt.headers}
			err := def.KeepRedacted(tt.stored)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("KeepRedacted = %v, want domain.ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeepRedacted: %v", err)
			}
			if !maps.Equal(def.Env, tt.wantEnv) || !maps.Equal(def.Headers, tt.wantHeaders) {
				t.Errorf("env %v, headers %v; want %v, %v", def.Env, def.Headers, tt.wantEnv, tt.wantHeaders)
			}
		})
	}
}

// KI-97: the url's password and arguments that carry a credential are
// redacted on reads too; a client that sends them back as read keeps the
// stored values, for the same transport, url, command and arguments only.

func TestServerDef_RedactedURLAndArgs(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		args     []string
		wantURL  string
		wantArgs []string
	}{
		// The url is redacted by secrets.RedactURLField (KI-97 review): the whole
		// userinfo (a token-only user too) and credential query values.
		{name: "password", url: "https://user:s3cret@mcp.example/sse?x=1", wantURL: "https://***@mcp.example/sse?x=1"},
		{name: "encoded password with @ and :", url: "https://user:p%40ss:w@mcp.example:8443/", wantURL: "https://***@mcp.example:8443/"},
		{name: "token as the user", url: "https://ghp_tok@mcp.example/", wantURL: "https://***@mcp.example/"},
		{name: "credential query parameters", url: "https://mcp.example/sse?api_key=sk-1&x=1&accessToken=a", wantURL: "https://mcp.example/sse?api_key=***&x=1&accessToken=***"},
		{name: "other query parameters stay", url: "https://mcp.example/sse?max_tokens=5&key=", wantURL: "https://mcp.example/sse?max_tokens=5&key="},
		{name: "no userinfo", url: "http://mcp.example/sse", wantURL: "http://mcp.example/sse"},
		// KI-97 security review: the url is one value, split as net/url does
		// (secrets.RedactURLField): quotes, brackets and spaces are part of the
		// userinfo or query value, and an @ after the authority never moves
		// the host that is shown.
		{name: "parentheses in the password", url: "https://svc:p(w)d@mcp.example/sse", wantURL: "https://***@mcp.example/sse"},
		{name: "parentheses in a query value", url: "https://mcp.example/sse?api_key=sk-ab(cd)ef", wantURL: "https://mcp.example/sse?api_key=***"},
		{name: "@ in the path", url: "http://mcp.example/a@b", wantURL: "http://mcp.example/a@b"},
		{name: "@ in the query", url: "https://evil.example/x?r=a@trusted.corp", wantURL: "https://evil.example/x?r=a@trusted.corp"},
		{name: "unparsable url", url: "https://user:p%zz@mcp.example/", wantURL: "https://***@mcp.example/"},
		{
			name:     "flag=value forms",
			args:     []string{"--token=ghp_1", "-api-key=k2", "PASSWORD=p3", "--githubToken=t4", "--port=8080", "--max-tokens=5", "--token="},
			wantArgs: []string{"--token=***", "-api-key=***", "PASSWORD=***", "--githubToken=***", "--port=8080", "--max-tokens=5", "--token="},
		},
		{
			name:     "flag value form",
			args:     []string{"--api-key", "k1", "-password", "p2", "--verbose", "--root", "/w", "--token"},
			wantArgs: []string{"--api-key", "***", "-password", "***", "--verbose", "--root", "/w", "--token"},
		},
		{
			name:     "a value that looks like a flag is still the value",
			args:     []string{"--secret", "--not-a-flag", "x"},
			wantArgs: []string{"--secret", "***", "x"},
		},
		{
			name:     "no credentials",
			args:     []string{"-y", "@modelcontextprotocol/server-filesystem", "/data", "token"},
			wantArgs: []string{"-y", "@modelcontextprotocol/server-filesystem", "/data", "token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := ServerDef{Transport: TransportSSE, URL: tt.url, Args: tt.args}
			got := stored.Redacted()
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
			if !slices.Equal(got.Args, tt.wantArgs) {
				t.Errorf("Args = %q, want %q", got.Args, tt.wantArgs)
			}
			if stored.URL != tt.url || !slices.Equal(stored.Args, tt.args) {
				t.Errorf("Redacted changed the stored definition: %+v", stored)
			}
		})
	}
}

// S7-G review: the env and the headers decide where a kept secret goes too
// (GITLAB_API_URL, HTTPS_PROXY, NODE_EXTRA_CA_CERTS, a header a proxy routes
// by). A RedactedValue is kept only while every other env entry and header is
// the stored one: none added, changed or removed.
func TestServerDef_KeepRedactedOnlyWithUnchangedEnvAndHeaders(t *testing.T) {
	stored := &ServerDef{
		Transport: TransportStreamableHTTP, URL: "https://gitlab.example/mcp",
		Env:     map[string]string{"GITLAB_TOKEN": "glpat-1", "GITLAB_API_URL": "https://gitlab.example/api/v4", "EMPTY": ""},
		Headers: map[string]string{"Authorization": "Bearer h", "X-Org": "acme"},
	}
	tests := []struct {
		name    string
		edit    func(d *ServerDef)
		wantErr bool
	}{
		{name: "sent back as read", edit: func(*ServerDef) {}},
		{name: "only the name changed", edit: func(d *ServerDef) { d.Name = "renamed" }},
		{
			name:    "env value changed",
			edit:    func(d *ServerDef) { d.Env["GITLAB_API_URL"] = "https://attacker.example/api/v4" },
			wantErr: true,
		},
		{
			name:    "env key added",
			edit:    func(d *ServerDef) { d.Env["HTTPS_PROXY"] = "http://attacker.example:3128" },
			wantErr: true,
		},
		{
			name:    "env key that loads a CA added",
			edit:    func(d *ServerDef) { d.Env["NODE_EXTRA_CA_CERTS"] = "/tmp/evil.pem" },
			wantErr: true,
		},
		{name: "env key removed", edit: func(d *ServerDef) { delete(d.Env, "GITLAB_API_URL") }, wantErr: true},
		{name: "empty env value set", edit: func(d *ServerDef) { d.Env["EMPTY"] = "x" }, wantErr: true},
		{name: "header value changed", edit: func(d *ServerDef) { d.Headers["X-Org"] = "evil" }, wantErr: true},
		{name: "header added", edit: func(d *ServerDef) { d.Headers["X-Forward-To"] = "evil" }, wantErr: true},
		{name: "header removed", edit: func(d *ServerDef) { delete(d.Headers, "X-Org") }, wantErr: true},
		{
			name: "env changed with every secret entered again",
			edit: func(d *ServerDef) {
				d.Env = map[string]string{"GITLAB_TOKEN": "glpat-2", "GITLAB_API_URL": "https://other.example/api/v4"}
				d.Headers = map[string]string{"Authorization": "Bearer new", "X-Org": "other"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := stored.Redacted()
			def.Env, def.Headers = maps.Clone(def.Env), maps.Clone(def.Headers)
			tt.edit(&def)
			before := def
			before.Env, before.Headers = maps.Clone(def.Env), maps.Clone(def.Headers)

			err := def.KeepRedacted(stored)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("KeepRedacted = %v, want domain.ErrValidation", err)
				}
				if !maps.Equal(def.Env, before.Env) || !maps.Equal(def.Headers, before.Headers) {
					t.Errorf("a refused request was changed: env %v, headers %v", def.Env, def.Headers)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeepRedacted: %v", err)
			}
			for k, v := range def.Env {
				if v == RedactedValue {
					t.Errorf("env %s still redacted", k)
				}
			}
			for k, v := range def.Headers {
				if v == RedactedValue {
					t.Errorf("header %s still redacted", k)
				}
			}
		})
	}
}

func TestServerDef_KeepRedactedURLAndArgs(t *testing.T) {
	stored := &ServerDef{
		Transport: TransportStdio, Command: "npx",
		Args: []string{"-y", "mcp-github", "--token=ghp_1", "--api-key", "k2", "--root", "/w"},
		Env:  map[string]string{"GITHUB_TOKEN": "env-secret"},
	}
	remote := &ServerDef{
		Transport: TransportStreamableHTTP, URL: "https://user:s3cret@mcp.example/mcp",
		Headers: map[string]string{"Authorization": "Bearer h"},
	}
	query := &ServerDef{Transport: TransportSSE, URL: "https://mcp.example/sse?api_key=sk-q&region=eu"}
	cascade := &ServerDef{Transport: TransportStdio, Command: "npx", Args: []string{"--token", "--api-key", "ghp_1"}}
	duplicates := &ServerDef{Transport: TransportStdio, Command: "npx", Args: []string{"--token", "ghp_1", "--token", "k2", "--token=ghp_1"}}
	// base is what the client read; stored is what the store holds (nil on create).
	tests := []struct {
		name     string
		base     *ServerDef
		stored   *ServerDef
		edit     func(d *ServerDef)
		wantErr  bool
		wantArgs []string
		wantURL  string
	}{
		{
			name: "args sent back as read", base: stored, stored: stored, edit: func(*ServerDef) {},
			wantArgs: stored.Args,
		},
		{
			name: "a new value replaces the stored one", base: stored, stored: stored,
			edit: func(d *ServerDef) { d.Args[2] = "--token=ghp_new" },
			// The other redacted value is kept, so the rest must stay as stored.
			wantErr: true,
		},
		{
			name: "all secrets entered again", base: stored, stored: stored,
			edit: func(d *ServerDef) {
				d.Args = []string{"-y", "mcp-github", "--token=ghp_new", "--api-key", "k_new", "--verbose"}
				d.Env = map[string]string{"GITHUB_TOKEN": "env-new"}
			},
			wantArgs: []string{"-y", "mcp-github", "--token=ghp_new", "--api-key", "k_new", "--verbose"},
		},
		{
			name: "changed flag name", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args[2] = "--password=***" },
			wantErr: true,
		},
		{
			name: "changed flag of a separate value", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args[3] = "--secret" },
			wantErr: true,
		},
		{
			name: "moved to another position", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args = append([]string{"--verbose"}, d.Args...) },
			wantErr: true,
		},
		{
			name: "another argument changed", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args[1] = "other-package" },
			wantErr: true,
		},
		{
			name: "redacted argument without a stored server", base: stored, stored: nil, edit: func(*ServerDef) {},
			wantErr: true,
		},
		{
			name: "url sent back as read", base: remote, stored: remote, edit: func(*ServerDef) {},
			wantURL: remote.URL,
		},
		{
			name: "url with another host", base: remote, stored: remote,
			edit:    func(d *ServerDef) { d.URL = "https://user:***@evil.example/mcp" },
			wantErr: true,
		},
		{
			name: "url with another path", base: remote, stored: remote,
			edit:    func(d *ServerDef) { d.URL = "https://user:***@mcp.example/other" },
			wantErr: true,
		},
		{
			name: "url with another user", base: remote, stored: remote,
			edit:    func(d *ServerDef) { d.URL = "https://admin:***@mcp.example/mcp" },
			wantErr: true,
		},
		{
			name: "url with another transport", base: remote, stored: remote,
			edit:    func(d *ServerDef) { d.Transport = TransportSSE },
			wantErr: true,
		},
		{
			name: "url with a new password", base: remote, stored: remote,
			edit: func(d *ServerDef) {
				d.URL = "https://user:new@mcp.example/mcp"
				d.Headers = map[string]string{"Authorization": "Bearer new"}
			},
			wantURL: "https://user:new@mcp.example/mcp",
		},
		{
			name: "redacted url without a stored password", base: remote, stored: &ServerDef{Transport: TransportStreamableHTTP, URL: "https://user@mcp.example/mcp"},
			edit:    func(d *ServerDef) { d.URL = "https://user:***@mcp.example/mcp" },
			wantErr: true,
		},
		{
			name: "redacted url on create", base: remote, stored: nil,
			edit:    func(d *ServerDef) { d.URL = "https://user:***@mcp.example/mcp" },
			wantErr: true,
		},
		{
			name: "url with a query credential sent back as read", base: query, stored: query, edit: func(*ServerDef) {},
			wantURL: query.URL,
		},
		{
			name: "url with a query credential and another parameter", base: query, stored: query,
			edit:    func(d *ServerDef) { d.URL = "https://mcp.example/sse?api_key=***&region=us" },
			wantErr: true,
		},
		// Round 2: the args are kept as a whole when they come back as read.
		{
			name: "a flag value that looks like a flag", base: cascade, stored: cascade, edit: func(*ServerDef) {},
			wantArgs: cascade.Args,
		},
		{
			name: "duplicate flags", base: duplicates, stored: duplicates, edit: func(*ServerDef) {},
			wantArgs: duplicates.Args,
		},
		{
			name: "an argument added", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args = append(d.Args, "--verbose") },
			wantErr: true,
		},
		{
			name: "an argument removed", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Args = d.Args[:len(d.Args)-1] },
			wantErr: true,
		},
		{
			name: "description edited", base: stored, stored: stored,
			edit:     func(d *ServerDef) { d.Description = "edited" },
			wantArgs: stored.Args,
		},
		{
			name: "another command with the args as read", base: stored, stored: stored,
			edit:    func(d *ServerDef) { d.Command = "/bin/other"; d.Env = nil },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := tt.base.Redacted()
			tt.edit(&def)
			sent := def.URL

			err := def.KeepRedacted(tt.stored)

			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("KeepRedacted = %v, want domain.ErrValidation", err)
				}
				if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "ghp_1") || strings.Contains(err.Error(), "sk-q") {
					t.Fatalf("the error quotes a secret: %v", err)
				}
				if def.URL != sent || slices.Contains(def.Args, "ghp_1") || slices.Contains(def.Args, "--token=ghp_1") ||
					slices.Contains(def.Args, "k2") {
					t.Fatalf("a refused definition carries stored secrets: %+v", def)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeepRedacted: %v", err)
			}
			if tt.wantArgs != nil && !slices.Equal(def.Args, tt.wantArgs) {
				t.Errorf("Args = %q, want %q", def.Args, tt.wantArgs)
			}
			if tt.wantURL != "" && def.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", def.URL, tt.wantURL)
			}
		})
	}
}

func TestServerDef_HasRedactedURLAndArgs(t *testing.T) {
	for _, tt := range []struct {
		def  ServerDef
		want bool
	}{
		{ServerDef{URL: "https://u:***@h/"}, true},
		{ServerDef{URL: "https://u:pw@h/"}, false},
		{ServerDef{Args: []string{"--token=***"}}, true},
		{ServerDef{Args: []string{"--token", "***"}}, true},
		// Round 2: any *** in the args or url stands for a value as read.
		{ServerDef{Args: []string{"--name", "***"}}, true},
		{ServerDef{Args: []string{"***"}}, true},
		{ServerDef{Args: []string{"--name", "**"}}, false},
		{ServerDef{URL: "https://h/?api_key=***"}, true},
	} {
		if got := tt.def.HasRedacted(); got != tt.want {
			t.Errorf("HasRedacted(%+v) = %v, want %v", tt.def, got, tt.want)
		}
	}
}
