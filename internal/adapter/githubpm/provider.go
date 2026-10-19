// Package githubpm implements a pmprovider.Provider for GitHub Issues through
// the GitHub REST API (KI-117: the Go Core runs no gh CLI).
package githubpm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Strob0t/CodeForge/internal/adapter/githubapi"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "github-issues"

// perPage and maxListPages bound a listing: up to 1,000 open issues.
const (
	perPage      = 100
	maxListPages = 10
)

// client is the HTTP client of every provider the factory creates: it
// connects through the PM outbound policy (SetOutboundPolicy) to
// api.github.com or an integration's GitHub Enterprise Server, never through
// a proxy, and follows redirects only within the origin, so the token stays
// with that API.
var client atomic.Pointer[http.Client]

// operatorToken is the operator's GitHub token (github.token): a provider
// without a token of its own uses it. The services let only the default
// tenant use it (KI-85); another tenant's integration brings its own.
var operatorToken atomic.Pointer[string]

func init() {
	client.Store(githubapi.PublicHTTPClient())
	SetOperatorToken("")
}

// SetOutboundPolicy makes the providers created from now on connect through
// policy; the Go Core builds it from pm.allowed_private_hosts at startup.
func SetOutboundPolicy(policy *netutil.OutboundPolicy) {
	client.Store(githubapi.NewHTTPClient(policy))
}

// SetOperatorToken sets the token of providers created from now on without
// one of their own; the Go Core sets github.token at startup.
func SetOperatorToken(token string) {
	operatorToken.Store(&token)
}

// Provider implements pmprovider.Provider for GitHub Issues via the REST API.
type Provider struct {
	token  string
	client *githubapi.Client
}

// newProvider returns a provider of the integration's configuration: its
// token, and the API of a GitHub Enterprise Server as base_url (KI-166;
// default api.github.com). Without a token it uses the operator's, which
// goes to api.github.com only.
func newProvider(cfg map[string]string) (*Provider, error) {
	token, baseURL := cfg["token"], strings.TrimSuffix(cfg["base_url"], "/")
	if baseURL == "" {
		baseURL = githubapi.DefaultBaseURL
	}
	if token == "" {
		if baseURL != githubapi.DefaultBaseURL {
			return nil, fmt.Errorf("%w: a github-issues base_url other than %s needs the integration's own token (github.token is sent to %s only)",
				domain.ErrValidation, githubapi.DefaultBaseURL, githubapi.DefaultBaseURL)
		}
		token = *operatorToken.Load()
	}
	return newProviderAt(baseURL, token, client.Load())
}

func newProviderAt(baseURL, token string, httpClient *http.Client) (*Provider, error) {
	c, err := githubapi.NewClient(baseURL, token, httpClient)
	if err != nil {
		return nil, err
	}
	return &Provider{token: token, client: c}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() pmprovider.Capabilities {
	return pmprovider.Capabilities{
		ListItems:  true,
		GetItem:    true,
		CreateItem: true,
		UpdateItem: true,
		Webhooks:   false,
	}
}

// ghIssue is an issue of the REST API.
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Assignees []ghUser  `json:"assignees"`
	// PullRequest is set for a pull request, which the issues endpoint
	// lists as well.
	PullRequest *json.RawMessage `json:"pull_request,omitempty"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghUser struct {
	Login string `json:"login"`
}

// ListItems returns the open issues of projectRef (owner/repo), pull requests
// left out, up to maxListPages pages.
func (p *Provider) ListItems(ctx context.Context, projectRef string) ([]pmprovider.Item, error) {
	repoPath, err := githubapi.RepoPath(projectRef)
	if err != nil {
		return nil, err
	}

	issues, more, err := githubapi.List[ghIssue](ctx, p.client, fmt.Sprintf("%s/issues?state=open&per_page=%d", repoPath, perPage), maxListPages)
	if err != nil {
		return nil, fmt.Errorf("github list issues: %w", err)
	}
	if more {
		slog.WarnContext(ctx, "github issues listing truncated", "repo", projectRef, "pages", maxListPages)
	}
	items := make([]pmprovider.Item, 0, len(issues))
	for i := range issues {
		if issues[i].PullRequest == nil {
			items = append(items, issueToItem(&issues[i], projectRef))
		}
	}
	return items, nil
}

func (p *Provider) GetItem(ctx context.Context, projectRef, itemID string) (*pmprovider.Item, error) {
	issuePath, err := issuePath(projectRef, itemID)
	if err != nil {
		return nil, err
	}
	page, err := p.client.Do(ctx, http.MethodGet, issuePath, nil)
	if err != nil {
		return nil, fmt.Errorf("github get issue: %w", err)
	}
	return decodeItem(page, projectRef)
}

// issueRequest is the body of an issue's creation or update; an empty body
// or label list is not sent (an update keeps the issue's).
type issueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

// CreateItem creates an issue with the item's title, description and labels.
func (p *Provider) CreateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	repoPath, err := githubapi.RepoPath(projectRef)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(issueRequest{Title: item.Title, Body: item.Description, Labels: item.Labels})
	if err != nil {
		return nil, fmt.Errorf("github marshal issue: %w", err)
	}
	page, err := p.client.Do(ctx, http.MethodPost, repoPath+"/issues", body)
	if err != nil {
		return nil, fmt.Errorf("github create issue: %w", err)
	}
	return decodeItem(page, projectRef)
}

// UpdateItem sets an issue's title and, when it has one, its description.
func (p *Provider) UpdateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	issuePath, err := issuePath(projectRef, item.ID)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(issueRequest{Title: item.Title, Body: item.Description})
	if err != nil {
		return nil, fmt.Errorf("github marshal issue: %w", err)
	}
	page, err := p.client.Do(ctx, http.MethodPatch, issuePath, body)
	if err != nil {
		return nil, fmt.Errorf("github update issue: %w", err)
	}
	return decodeItem(page, projectRef)
}

func decodeItem(page *githubapi.Page, projectRef string) (*pmprovider.Item, error) {
	issue, err := githubapi.Decode[ghIssue](page.Body)
	if err != nil {
		return nil, err
	}
	item := issueToItem(&issue, projectRef)
	return &item, nil
}

// issuePath returns the API path of issue itemID (a positive number) of
// projectRef.
func issuePath(projectRef, itemID string) (string, error) {
	repoPath, err := githubapi.RepoPath(projectRef)
	if err != nil {
		return "", err
	}
	number, err := strconv.ParseUint(itemID, 10, 31)
	if err != nil || number == 0 || strconv.FormatUint(number, 10) != itemID {
		return "", fmt.Errorf("%w: invalid issue number %q", domain.ErrValidation, itemID)
	}
	return repoPath + "/issues/" + itemID, nil
}

func issueToItem(issue *ghIssue, repo string) pmprovider.Item {
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}

	assignee := ""
	if len(issue.Assignees) > 0 {
		assignee = issue.Assignees[0].Login
	}

	return pmprovider.Item{
		ID:          strconv.Itoa(issue.Number),
		ExternalID:  fmt.Sprintf("%s#%d", repo, issue.Number),
		Title:       issue.Title,
		Description: issue.Body,
		Status:      strings.ToLower(issue.State),
		Labels:      labels,
		Assignee:    assignee,
	}
}
