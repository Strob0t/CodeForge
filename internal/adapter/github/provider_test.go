package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/githubapi"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// loopbackProvider is a provider of the API at baseURL whose outbound policy
// allows the loopback address httptest listens on.
func loopbackProvider(t *testing.T, token, baseURL string) *Provider {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	return newProvider(token, baseURL, githubapi.NewHTTPClient(policy))
}

func TestName(t *testing.T) {
	p := NewProvider("tok", "")
	if got := p.Name(); got != "github-api" {
		t.Fatalf("Name() = %q, want %q", got, "github-api")
	}
}

func TestCapabilities(t *testing.T) {
	p := NewProvider("tok", "")
	c := p.Capabilities()
	if !c.Clone || !c.Push || !c.PullRequest || !c.Webhook || !c.Issues {
		t.Fatalf("unexpected capabilities: %+v", c)
	}
}

func TestCloneURL(t *testing.T) {
	p := NewProvider("my-token", "")
	url, err := p.CloneURL(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("CloneURL() error: %v", err)
	}
	expected := "https://x-access-token:my-token@github.com/owner/repo.git"
	if url != expected {
		t.Fatalf("CloneURL() = %q, want %q", url, expected)
	}
}

func TestCloneURL_Empty(t *testing.T) {
	p := NewProvider("tok", "")
	_, err := p.CloneURL(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty repo")
	}
}

func TestCloneURL_CustomBaseURL(t *testing.T) {
	p := NewProvider("tok", "https://ghe.example.com/api/v3")
	url, err := p.CloneURL(context.Background(), "org/repo")
	if err != nil {
		t.Fatalf("CloneURL() error: %v", err)
	}
	expected := "https://x-access-token:tok@ghe.example.com/org/repo.git"
	if url != expected {
		t.Fatalf("CloneURL() = %q, want %q", url, expected)
	}
}

func TestListRepos(t *testing.T) {
	repos := []ghRepo{
		{FullName: "owner/repo1"},
		{FullName: "owner/repo2"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer srv.Close()

	p := loopbackProvider(t, "test-token", srv.URL)
	got, err := p.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2", len(got))
	}
	if got[0] != "owner/repo1" || got[1] != "owner/repo2" {
		t.Fatalf("ListRepos() = %v, want [owner/repo1, owner/repo2]", got)
	}
}

func TestListRepos_Pagination(t *testing.T) {
	page1 := []ghRepo{{FullName: "a/1"}}
	page2 := []ghRepo{{FullName: "b/2"}}

	// We need srv.URL inside the handler, so use a pointer to hold it.
	var srvURL string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.Header().Set("Link", `<`+srvURL+`/page2>; rel="next"`)
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	p := loopbackProvider(t, "tok", srv.URL)
	got, err := p.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2", len(got))
	}
	if got[0] != "a/1" || got[1] != "b/2" {
		t.Fatalf("ListRepos() = %v, want [a/1, b/2]", got)
	}
}

func TestListRepos_StaysOnTheAPIOrigin(t *testing.T) {
	var other int
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		other++
		_ = json.NewEncoder(w).Encode([]ghRepo{{FullName: "x/y"}})
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `<`+strings.Replace(elsewhere.URL, "127.0.0.1", "localhost", 1)+`/page2>; rel="next"`)
		_ = json.NewEncoder(w).Encode([]ghRepo{{FullName: "a/1"}})
	}))
	defer srv.Close()

	got, err := loopbackProvider(t, "tok", srv.URL).ListRepos(context.Background())
	if err != nil || len(got) != 1 || other != 0 {
		t.Fatalf("ListRepos() = %v, %v; requests elsewhere %d", got, err, other)
	}
}

func TestProviderOpensPullRequests(t *testing.T) {
	prov, err := gitprovider.New("github-api", map[string]string{"token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prov.(gitprovider.PullRequestCreator); !ok {
		t.Fatalf("%T does not open pull requests", prov)
	}
}

type prRequest struct {
	method, path, auth string
	body               map[string]string
}

// fakePullsAPI answers GET /repos/acme/app with the default branch and
// POST /repos/acme/app/pulls with status and answer.
func fakePullsAPI(t *testing.T, status int, answer string) (*httptest.Server, *[]prRequest) {
	t.Helper()
	var requests []prRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := prRequest{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization")}
		if data, _ := io.ReadAll(r.Body); len(data) > 0 {
			if err := json.Unmarshal(data, &req.body); err != nil {
				t.Errorf("body %s: %v", data, err)
			}
		}
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app":
			_, _ = io.WriteString(w, `{"full_name":"acme/app","default_branch":"develop"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/app/pulls":
			w.WriteHeader(status)
			_, _ = io.WriteString(w, answer)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

// KI-117: PR delivery opens the pull request through the REST API, into the
// repository's default branch unless a base is given.
func TestCreatePullRequest(t *testing.T) {
	srv, requests := fakePullsAPI(t, http.StatusCreated, `{"number":7,"html_url":"https://github.com/acme/app/pull/7"}`)

	url, err := loopbackProvider(t, "ghp_tok", srv.URL).CreatePullRequest(context.Background(), &gitprovider.PullRequest{
		Repo: "acme/app", Head: "codeforge/abcd1234", Title: "codeforge: add feature", Body: "Automated delivery",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if url != "https://github.com/acme/app/pull/7" {
		t.Fatalf("url %q", url)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests %+v", *requests)
	}
	post := (*requests)[1]
	want := map[string]string{"title": "codeforge: add feature", "head": "codeforge/abcd1234", "base": "develop", "body": "Automated delivery"}
	for k, v := range want {
		if post.body[k] != v {
			t.Errorf("body[%s] = %q, want %q", k, post.body[k], v)
		}
	}
	for _, req := range *requests {
		if req.auth != "Bearer ghp_tok" {
			t.Errorf("%s %s: Authorization %q", req.method, req.path, req.auth)
		}
	}
}

func TestCreatePullRequest_GivenBase(t *testing.T) {
	srv, requests := fakePullsAPI(t, http.StatusCreated, `{"html_url":"https://github.com/acme/app/pull/8"}`)

	if _, err := loopbackProvider(t, "tok", srv.URL).CreatePullRequest(context.Background(), &gitprovider.PullRequest{
		Repo: "acme/app", Head: "codeforge/x", Base: "release", Title: "t",
	}); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || (*requests)[0].body["base"] != "release" {
		t.Fatalf("requests %+v", *requests)
	}
}

func TestCreatePullRequest_Errors(t *testing.T) {
	tests := []struct {
		name   string
		repo   string
		status int
		answer string
		wantIs error
		want   string
	}{
		{"bad token", "acme/app", http.StatusUnauthorized, `{"message":"Bad credentials"}`, domain.ErrValidation, "Bad credentials"},
		{"unknown repository", "acme/gone", http.StatusCreated, `{}`, domain.ErrNotFound, "404"},
		{"pull request exists", "acme/app", http.StatusUnprocessableEntity,
			`{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"custom","message":"A pull request already exists for acme:codeforge/x."}]}`,
			domain.ErrValidation, "A pull request already exists"},
		{"no html_url", "acme/app", http.StatusCreated, `{"number":1}`, nil, "no pull request URL"},
		{"invalid repository", "acme/../x", http.StatusCreated, `{}`, domain.ErrValidation, "invalid repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := fakePullsAPI(t, tt.status, tt.answer)
			_, err := loopbackProvider(t, "ghp_secret", srv.URL).CreatePullRequest(context.Background(), &gitprovider.PullRequest{
				Repo: tt.repo, Head: "codeforge/x", Title: "t",
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err %v, want it to mention %q", err, tt.want)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err %v, want %v", err, tt.wantIs)
			}
			if strings.Contains(err.Error(), "ghp_secret") {
				t.Fatalf("err %q leaks the token", err)
			}
		})
	}
}

func TestCreatePullRequest_NeedsHeadAndTitle(t *testing.T) {
	p := NewProvider("tok", "")
	for _, pr := range []gitprovider.PullRequest{{Repo: "a/b", Title: "t"}, {Repo: "a/b", Head: "h"}} {
		if _, err := p.CreatePullRequest(context.Background(), &pr); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("CreatePullRequest(%+v) = %v, want ErrValidation", pr, err)
		}
	}
}
