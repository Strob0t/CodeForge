package githubpm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/githubapi"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const testToken = "ghp_INTEGRATION-secret-1234"

// fakeGitHub records the requests of a test and answers them with handle.
type fakeGitHub struct {
	mu       sync.Mutex
	requests []recordedRequest
	srv      *httptest.Server
}

type recordedRequest struct {
	Method, Path, Query, Auth, Accept, APIVersion string
	Body                                          map[string]json.RawMessage
}

func newFakeGitHub(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"), APIVersion: r.Header.Get("X-GitHub-Api-Version"),
		}
		if data, _ := io.ReadAll(r.Body); len(data) > 0 {
			if err := json.Unmarshal(data, &rec.Body); err != nil {
				t.Errorf("request body is not a JSON object: %s", data)
			}
		}
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		f.mu.Unlock()
		handle(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

// provider returns a provider of the fake API with token; its policy allows
// the loopback address httptest listens on.
func (f *fakeGitHub) provider(t *testing.T, token string) *Provider {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := newProviderAt(f.srv.URL, token, githubapi.NewHTTPClient(policy))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func writeJSON[T any](t *testing.T, w http.ResponseWriter, status int, v T) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}

// apiIssue is an issue as the REST API answers it.
type apiIssue struct {
	Number      int               `json:"number"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	State       string            `json:"state"`
	Labels      []ghLabel         `json:"labels"`
	Assignees   []ghUser          `json:"assignees"`
	PullRequest map[string]string `json:"pull_request,omitempty"`
}

func issueJSON(number int, title, state string) apiIssue {
	return apiIssue{
		Number: number, Title: title, Body: "body " + title, State: state,
		Labels: []ghLabel{{Name: "bug"}}, Assignees: []ghUser{{Login: "alice"}},
	}
}

func TestIssueToItem(t *testing.T) {
	issue := &ghIssue{
		Number:    42,
		Title:     "Fix login bug",
		Body:      "The login form crashes",
		State:     "OPEN",
		Labels:    []ghLabel{{Name: "bug"}, {Name: "priority:high"}},
		Assignees: []ghUser{{Login: "alice"}},
	}

	item := issueToItem(issue, "owner/repo")

	if item.ID != "42" || item.ExternalID != "owner/repo#42" || item.Title != "Fix login bug" {
		t.Errorf("item = %+v", item)
	}
	if item.Status != "open" || len(item.Labels) != 2 || item.Assignee != "alice" {
		t.Errorf("item = %+v", item)
	}
}

func TestIssueToItem_NoAssignee(t *testing.T) {
	item := issueToItem(&ghIssue{Number: 1, Title: "Test", State: "closed"}, "org/proj")
	if item.Assignee != "" || item.Status != "closed" {
		t.Errorf("item = %+v", item)
	}
}

func TestGitHubPM_ProviderNameAndCapabilities(t *testing.T) {
	p, err := pmprovider.New(providerName, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "github-issues" {
		t.Fatalf("name %q", p.Name())
	}
	caps := p.Capabilities()
	if !caps.ListItems || !caps.GetItem || !caps.CreateItem || !caps.UpdateItem || caps.Webhooks {
		t.Fatalf("capabilities %+v", caps)
	}
}

// KI-117: the open issues through the REST API, every page, pull requests
// (which the issues endpoint lists too) left out.
func TestListItems_PaginatesAndSkipsPullRequests(t *testing.T) {
	var f *fakeGitHub
	f = newFakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/repositories/7/issues?state=open&per_page=100&page=2>; rel="next", <%s/repositories/7/issues?page=2>; rel="last"`, f.srv.URL, f.srv.URL))
			pr := issueJSON(3, "a pull request", "open")
			pr.PullRequest = map[string]string{"url": "x"}
			writeJSON(t, w, http.StatusOK, []apiIssue{issueJSON(1, "one", "open"), pr, issueJSON(2, "two", "open")})
		case "2":
			writeJSON(t, w, http.StatusOK, []apiIssue{issueJSON(4, "four", "open")})
		default:
			t.Errorf("unexpected page %q", r.URL.RawQuery)
		}
	})

	items, err := f.provider(t, testToken).ListItems(t.Context(), "acme/app")
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	var ids []string
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if strings.Join(ids, ",") != "1,2,4" {
		t.Fatalf("items %v, want issues 1, 2 and 4", ids)
	}
	if items[0].ExternalID != "acme/app#1" || items[0].Labels[0] != "bug" || items[0].Assignee != "alice" || items[0].Status != "open" {
		t.Errorf("item %+v", items[0])
	}
	reqs := f.recorded()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	first := reqs[0]
	if first.Method != http.MethodGet || first.Path != "/repos/acme/app/issues" || first.Query != "state=open&per_page=100" {
		t.Errorf("first request %+v", first)
	}
	for _, req := range reqs {
		if req.Auth != "Bearer "+testToken || req.Accept != "application/vnd.github+json" || req.APIVersion == "" {
			t.Errorf("request headers %+v", req)
		}
	}
}

// The token goes with every page: a next link to another origin is not
// followed.
func TestListItems_NextLinkToAnotherOriginIsNotFollowed(t *testing.T) {
	other := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []apiIssue{issueJSON(9, "elsewhere", "open")})
	})
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/app/issues?page=2>; rel="next"`, strings.Replace(other.srv.URL, "127.0.0.1", "localhost", 1)))
		writeJSON(t, w, http.StatusOK, []apiIssue{issueJSON(1, "one", "open")})
	})

	items, err := f.provider(t, testToken).ListItems(t.Context(), "acme/app")
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 || len(other.recorded()) != 0 {
		t.Fatalf("items %d, requests to the other origin %d", len(items), len(other.recorded()))
	}
}

func TestListItems_StopsAfterMaxPages(t *testing.T) {
	var f *fakeGitHub
	f = newFakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/acme/app/issues?page=%d>; rel="next"`, f.srv.URL, len(f.recorded())+1))
		writeJSON(t, w, http.StatusOK, []apiIssue{issueJSON(len(f.recorded()), "n", "open")})
	})

	items, err := f.provider(t, testToken).ListItems(t.Context(), "acme/app")
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(f.recorded()) != maxListPages || len(items) != maxListPages {
		t.Fatalf("%d requests, %d items, want %d", len(f.recorded()), len(items), maxListPages)
	}
}

func TestGetItem(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, issueJSON(42, "Test", "closed"))
	})

	item, err := f.provider(t, testToken).GetItem(t.Context(), "acme/app", "42")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.ID != "42" || item.Title != "Test" || item.Status != "closed" {
		t.Errorf("item %+v", item)
	}
	if req := f.recorded()[0]; req.Method != http.MethodGet || req.Path != "/repos/acme/app/issues/42" {
		t.Errorf("request %+v", req)
	}
}

func TestItemIDIsANumber(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request for an invalid item ID")
		w.WriteHeader(http.StatusTeapot)
	})
	p := f.provider(t, testToken)
	for _, id := range []string{"", "0", "-1", "../../user", "42/comments", "4 2", "1e3", "99999999999999999999"} {
		if _, err := p.GetItem(t.Context(), "acme/app", id); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("GetItem(%q) = %v, want ErrValidation", id, err)
		}
		if _, err := p.UpdateItem(t.Context(), "acme/app", &pmprovider.Item{ID: id, Title: "t"}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("UpdateItem(%q) = %v, want ErrValidation", id, err)
		}
	}
}

func TestCreateItem(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, issueJSON(99, "New bug", "open"))
	})

	created, err := f.provider(t, testToken).CreateItem(t.Context(), "acme/app", &pmprovider.Item{
		Title: "New bug", Description: "Something broke", Labels: []string{"bug"},
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if created.ID != "99" || created.ExternalID != "acme/app#99" {
		t.Errorf("created %+v", created)
	}
	req := f.recorded()[0]
	if req.Method != http.MethodPost || req.Path != "/repos/acme/app/issues" {
		t.Errorf("request %+v", req)
	}
	if string(req.Body["title"]) != `"New bug"` || string(req.Body["body"]) != `"Something broke"` || string(req.Body["labels"]) != `["bug"]` {
		t.Errorf("request body %v", req.Body)
	}
}

func TestCreateItem_WithoutDescriptionOrLabels(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, issueJSON(5, "t", "open"))
	})
	if _, err := f.provider(t, testToken).CreateItem(t.Context(), "acme/app", &pmprovider.Item{Title: "t"}); err != nil {
		t.Fatal(err)
	}
	body := f.recorded()[0].Body
	if _, ok := body["body"]; ok {
		t.Errorf("an empty description is not sent: %v", body)
	}
	if _, ok := body["labels"]; ok {
		t.Errorf("no labels are not sent: %v", body)
	}
}

func TestUpdateItem(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, issueJSON(42, "Updated title", "open"))
	})

	updated, err := f.provider(t, testToken).UpdateItem(t.Context(), "acme/app", &pmprovider.Item{
		ID: "42", Title: "Updated title", Description: "Updated body",
	})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if updated.ID != "42" || updated.Title != "Updated title" {
		t.Errorf("updated %+v", updated)
	}
	req := f.recorded()[0]
	if req.Method != http.MethodPatch || req.Path != "/repos/acme/app/issues/42" {
		t.Errorf("request %+v", req)
	}
	if string(req.Body["title"]) != `"Updated title"` || string(req.Body["body"]) != `"Updated body"` {
		t.Errorf("request body %v", req.Body)
	}
}

func TestInvalidRefSendsNothing(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request for an invalid project ref")
		w.WriteHeader(http.StatusTeapot)
	})
	p := f.provider(t, testToken)
	ctx := t.Context()
	if _, err := p.ListItems(ctx, "invalid"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("ListItems: %v", err)
	}
	if _, err := p.GetItem(ctx, "a/b/c", "1"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("GetItem: %v", err)
	}
	if _, err := p.CreateItem(ctx, "", &pmprovider.Item{Title: "t"}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("CreateItem: %v", err)
	}
	if _, err := p.UpdateItem(ctx, "owner/..", &pmprovider.Item{ID: "1", Title: "t"}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("UpdateItem: %v", err)
	}
}

// API errors: the caller gets the status and GitHub's message, a 404 is not
// found, a rejected token or field is the caller's to correct (400), and no
// error carries the token or the API's URL.
func TestAPIErrors(t *testing.T) {
	tests := []struct {
		status     int
		body       string
		wantIs     error
		wantSubstr []string
	}{
		{http.StatusUnauthorized, `{"message":"Bad credentials","documentation_url":"https://docs.github.com"}`, domain.ErrValidation,
			[]string{"401", "the token is missing, invalid or expired", "Bad credentials"}},
		{http.StatusForbidden, `{"message":"Resource not accessible by personal access token"}`, domain.ErrValidation,
			[]string{"403", "Resource not accessible"}},
		{http.StatusNotFound, `{"message":"Not Found"}`, domain.ErrNotFound, []string{"404"}},
		{http.StatusUnprocessableEntity, `{"message":"Validation Failed","errors":[{"resource":"Issue","code":"missing_field","field":"title"}]}`,
			domain.ErrValidation, []string{"422", "Validation Failed", "title missing_field"}},
		{http.StatusInternalServerError, `<html>oops</html>`, nil, []string{"500"}},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			p := f.provider(t, testToken)
			calls := map[string]func() error{
				"list": func() error { _, err := p.ListItems(t.Context(), "acme/app"); return err },
				"get":  func() error { _, err := p.GetItem(t.Context(), "acme/app", "1"); return err },
				"create": func() error {
					_, err := p.CreateItem(t.Context(), "acme/app", &pmprovider.Item{Title: "t"})
					return err
				},
				"update": func() error {
					_, err := p.UpdateItem(t.Context(), "acme/app", &pmprovider.Item{ID: "1", Title: "t"})
					return err
				},
			}
			for name, call := range calls {
				err := call()
				if err == nil {
					t.Fatalf("%s: no error", name)
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("%s: %v, want %v", name, err, tt.wantIs)
				}
				if tt.wantIs == nil && (errors.Is(err, domain.ErrValidation) || errors.Is(err, domain.ErrNotFound)) {
					t.Errorf("%s: a server error is not the caller's: %v", name, err)
				}
				for _, s := range tt.wantSubstr {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("%s: %q does not mention %q", name, err, s)
					}
				}
				if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), f.srv.URL) || strings.Contains(err.Error(), "oops") {
					t.Errorf("%s: %q leaks the token, the API URL or the answer", name, err)
				}
			}
		})
	}
}

// KI-85, KI-117: a PM integration's own token authenticates its requests;
// without one, the operator's GitHub token (github.token) does, which the
// services allow only in the default tenant; without either, the requests
// are anonymous (public repositories).
func TestTokenChoice(t *testing.T) {
	f := newFakeGitHub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, []apiIssue{})
	})
	tests := []struct {
		name, integration, operator, wantAuth string
	}{
		{"integration token", testToken, "ghp_operator", "Bearer " + testToken},
		{"operator token", "", "ghp_operator", "Bearer ghp_operator"},
		{"anonymous", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetOperatorToken(tt.operator)
			t.Cleanup(func() { SetOperatorToken("") })
			prov, err := pmprovider.New(providerName, map[string]string{"token": tt.integration})
			if err != nil {
				t.Fatal(err)
			}
			p, ok := prov.(*Provider)
			if !ok {
				t.Fatalf("provider %T", prov)
			}
			// The registered factory's client, pointed at the fake API.
			test := f.provider(t, p.token)
			if _, err := test.ListItems(t.Context(), "acme/app"); err != nil {
				t.Fatal(err)
			}
			reqs := f.recorded()
			if got := reqs[len(reqs)-1].Auth; got != tt.wantAuth {
				t.Fatalf("Authorization %q, want %q", got, tt.wantAuth)
			}
		})
	}
}

// The default API is github.com's, through the outbound policy.
func TestProviderUsesGitHubAPI(t *testing.T) {
	prov, err := pmprovider.New(providerName, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := prov.(*Provider).client.BaseURL(); got != githubapi.DefaultBaseURL {
		t.Fatalf("base URL %q", got)
	}
}
