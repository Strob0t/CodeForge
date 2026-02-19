// Package gitlab implements a pmprovider.Provider for GitLab instances using their REST API v4.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "gitlab"

// Provider implements pmprovider.Provider for GitLab Issues via the REST API v4.
type Provider struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewProvider creates a GitLab provider with the given base URL and private token.
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

// gitlabIssue mirrors the JSON response from the GitLab issues API.
type gitlabIssue struct {
	IID         int          `json:"iid"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	State       string       `json:"state"`
	Labels      []string     `json:"labels"`
	Assignees   []gitlabUser `json:"assignees"`
}

type gitlabUser struct {
	Username string `json:"username"`
}

func (p *Provider) ListItems(ctx context.Context, projectRef string) ([]pmprovider.Item, error) {
	encodedRef := url.PathEscape(projectRef)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues?per_page=50&state=opened", p.baseURL, encodedRef)
	body, err := p.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab list issues: %w", err)
	}

	var issues []gitlabIssue
	if err := json.Unmarshal(body, &issues); err != nil {
		return nil, fmt.Errorf("gitlab parse response: %w", err)
	}

	items := make([]pmprovider.Item, 0, len(issues))
	for i := range issues {
		items = append(items, issueToItem(&issues[i], projectRef))
	}
	return items, nil
}

func (p *Provider) GetItem(ctx context.Context, projectRef, itemID string) (*pmprovider.Item, error) {
	encodedRef := url.PathEscape(projectRef)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues/%s", p.baseURL, encodedRef, itemID)
	body, err := p.doRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab get issue: %w", err)
	}

	var issue gitlabIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, fmt.Errorf("gitlab parse response: %w", err)
	}

	item := issueToItem(&issue, projectRef)
	return &item, nil
}

func (p *Provider) CreateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	encodedRef := url.PathEscape(projectRef)

	payload := map[string]string{
		"title":       item.Title,
		"description": item.Description,
	}
	payloadJSON, _ := json.Marshal(payload)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues", p.baseURL, encodedRef)
	body, err := p.doRequest(ctx, http.MethodPost, reqURL, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitlab create issue: %w", err)
	}

	var created gitlabIssue
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, fmt.Errorf("gitlab parse response: %w", err)
	}

	result := issueToItem(&created, projectRef)
	return &result, nil
}

func (p *Provider) UpdateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	encodedRef := url.PathEscape(projectRef)

	payload := map[string]string{
		"title":       item.Title,
		"description": item.Description,
	}
	if item.Status != "" {
		// GitLab uses "close" / "reopen" as state_event, but also accepts "closed" / "opened" on state.
		payload["state_event"] = mapStatusToStateEvent(item.Status)
	}
	payloadJSON, _ := json.Marshal(payload)

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues/%s", p.baseURL, encodedRef, item.ID)
	body, err := p.doRequest(ctx, http.MethodPut, reqURL, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitlab update issue: %w", err)
	}

	var updated gitlabIssue
	if err := json.Unmarshal(body, &updated); err != nil {
		return nil, fmt.Errorf("gitlab parse response: %w", err)
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
		req.Header.Set("PRIVATE-TOKEN", p.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.httpClient.Do(req) //nolint:gosec // URL is constructed from trusted baseURL + project ref
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gitlab API %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func issueToItem(issue *gitlabIssue, projectRef string) pmprovider.Item {
	labels := make([]string, len(issue.Labels))
	copy(labels, issue.Labels)

	assignee := ""
	if len(issue.Assignees) > 0 {
		assignee = issue.Assignees[0].Username
	}

	return pmprovider.Item{
		ID:          fmt.Sprintf("%d", issue.IID),
		ExternalID:  fmt.Sprintf("%s#%d", projectRef, issue.IID),
		Title:       issue.Title,
		Description: issue.Description,
		Status:      strings.ToLower(issue.State),
		Labels:      labels,
		Assignee:    assignee,
	}
}

// mapStatusToStateEvent converts a pmprovider status to a GitLab state_event value.
func mapStatusToStateEvent(status string) string {
	switch strings.ToLower(status) {
	case "closed":
		return "close"
	case "opened", "open":
		return "reopen"
	default:
		return status
	}
}
