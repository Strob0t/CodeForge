// Package gitea implements a pmprovider.Provider for Gitea/Forgejo instances using their REST API.
package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "gitea"

// Provider implements pmprovider.Provider for Gitea/Forgejo Issues via their REST API.
type Provider struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewProvider creates a Gitea provider with the given base URL and token.
func NewProvider(baseURL, token string) *Provider {
	return &Provider{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		httpClient: http.DefaultClient,
	}
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

// giteaIssue mirrors the JSON response from the Gitea issues API.
type giteaIssue struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	State     string       `json:"state"`
	Labels    []giteaLabel `json:"labels"`
	Assignees []giteaUser  `json:"assignees"`
}

type giteaLabel struct {
	Name string `json:"name"`
}

type giteaUser struct {
	Login string `json:"login"`
}

func (p *Provider) ListItems(ctx context.Context, projectRef string) ([]pmprovider.Item, error) {
	owner, repo, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues?type=issues&limit=50&state=open", p.baseURL, owner, repo)
	body, err := p.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gitea list issues: %w", err)
	}

	var issues []giteaIssue
	if err := json.Unmarshal(body, &issues); err != nil {
		return nil, fmt.Errorf("gitea parse response: %w", err)
	}

	items := make([]pmprovider.Item, 0, len(issues))
	for i := range issues {
		items = append(items, issueToItem(&issues[i], projectRef))
	}
	return items, nil
}

func (p *Provider) GetItem(ctx context.Context, projectRef, itemID string) (*pmprovider.Item, error) {
	owner, repo, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%s", p.baseURL, owner, repo, itemID)
	body, err := p.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gitea get issue: %w", err)
	}

	var issue giteaIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, fmt.Errorf("gitea parse response: %w", err)
	}

	item := issueToItem(&issue, projectRef)
	return &item, nil
}

// CreateItem creates a new issue in the Gitea repository.
func (p *Provider) CreateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	owner, repo, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	payload := map[string]string{
		"title": item.Title,
		"body":  item.Description,
	}
	payloadJSON, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues", p.baseURL, owner, repo)
	body, err := p.doRequest(ctx, http.MethodPost, url, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitea create issue: %w", err)
	}

	var created giteaIssue
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, fmt.Errorf("gitea parse response: %w", err)
	}

	result := issueToItem(&created, projectRef)
	return &result, nil
}

// UpdateItem updates an existing issue in the Gitea repository.
func (p *Provider) UpdateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	owner, repo, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	payload := map[string]string{
		"title": item.Title,
		"body":  item.Description,
	}
	if item.Status != "" {
		payload["state"] = item.Status
	}
	payloadJSON, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%s", p.baseURL, owner, repo, item.ID)
	body, err := p.doRequest(ctx, http.MethodPatch, url, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitea update issue: %w", err)
	}

	var updated giteaIssue
	if err := json.Unmarshal(body, &updated); err != nil {
		return nil, fmt.Errorf("gitea parse response: %w", err)
	}

	result := issueToItem(&updated, projectRef)
	return &result, nil
}

func (p *Provider) doRequest(ctx context.Context, method, reqURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "token "+p.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL is constructed from trusted baseURL + owner/repo
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gitea API %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func issueToItem(issue *giteaIssue, projectRef string) pmprovider.Item {
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}

	assignee := ""
	if len(issue.Assignees) > 0 {
		assignee = issue.Assignees[0].Login
	}

	return pmprovider.Item{
		ID:          fmt.Sprintf("%d", issue.Number),
		ExternalID:  fmt.Sprintf("%s#%d", projectRef, issue.Number),
		Title:       issue.Title,
		Description: issue.Body,
		Status:      strings.ToLower(issue.State),
		Labels:      labels,
		Assignee:    assignee,
	}
}

func parseProjectRef(ref string) (owner, repo string, err error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid project ref %q: expected owner/repo", ref)
	}
	return parts[0], parts[1], nil
}
