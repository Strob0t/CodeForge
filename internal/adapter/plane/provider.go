// Package plane implements a pmprovider.Provider for Plane.so using the HTTP REST API.
package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "plane"

// Provider implements pmprovider.Provider for Plane.so via the REST API.
type Provider struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

// newProvider creates a Provider from a config map.
// Config keys: "api_token", "base_url" (optional, defaults to "https://api.plane.so").
// The api_token should be provided via the config map (sourced from cfg.Plane.APIToken
// which reads CODEFORGE_PLANE_API_TOKEN via the config loader).
func newProvider(config map[string]string) (*Provider, error) {
	token := config["api_token"]
	if token == "" {
		return nil, fmt.Errorf("plane: api_token is required (set CODEFORGE_PLANE_API_TOKEN or plane.api_token in config)")
	}

	baseURL := config["base_url"]
	if baseURL == "" {
		baseURL = "https://api.plane.so"
	}

	return &Provider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   token,
		httpClient: &http.Client{},
	}, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() pmprovider.Capabilities {
	return pmprovider.Capabilities{
		ListItems:  true,
		GetItem:    true,
		CreateItem: true,
		UpdateItem: true,
		Webhooks:   true,
	}
}

// parseProjectRef splits "workspace-slug/project-id" into its two parts.
func parseProjectRef(ref string) (workspace, projectID string, err error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("plane: invalid project ref %q: expected workspace-slug/project-id", ref)
	}
	return parts[0], parts[1], nil
}

// issuesURL returns the base issues endpoint for a workspace/project.
func (p *Provider) issuesURL(workspace, projectID string) string {
	return fmt.Sprintf("%s/api/v1/workspaces/%s/projects/%s/issues/", p.baseURL, workspace, projectID)
}

// doRequest creates and executes an HTTP request with the Plane API authentication header.
func (p *Provider) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("plane: create request: %w", err)
	}
	req.Header.Set("X-API-Key", p.apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return p.httpClient.Do(req)
}

// planeIssueResponse represents a single issue from the Plane API.
type planeIssueResponse struct {
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	DescriptionStripped string                   `json:"description_stripped"`
	StateDetail         planeStateDetailResponse `json:"state_detail"`
	LabelDetails        []planeLabelResponse     `json:"label_details"`
	AssigneeDetails     []planeAssigneeResponse  `json:"assignee_details"`
	Priority            string                   `json:"priority"`
}

type planeStateDetailResponse struct {
	Group string `json:"group"`
}

type planeLabelResponse struct {
	Name string `json:"name"`
}

type planeAssigneeResponse struct {
	DisplayName string `json:"display_name"`
}

// planeListResponseBody represents the paginated list response from the Plane API.
type planeListResponseBody struct {
	Results         []planeIssueResponse `json:"results"`
	NextCursor      string               `json:"next_cursor"`
	NextPageResults bool                 `json:"next_page_results"`
}

// mapStatus converts Plane state group names to CodeForge feature statuses.
func mapStatus(group string) string {
	switch group {
	case "backlog":
		return "backlog"
	case "unstarted":
		return "planned"
	case "started":
		return "in_progress"
	case "completed":
		return "done"
	case "cancelled":
		return "cancelled"
	default:
		return group
	}
}

// issueToItem converts a Plane API issue response to a pmprovider.Item.
func issueToItem(issue *planeIssueResponse, projectRef string) pmprovider.Item {
	labels := make([]string, 0, len(issue.LabelDetails))
	for _, l := range issue.LabelDetails {
		labels = append(labels, l.Name)
	}

	assignee := ""
	if len(issue.AssigneeDetails) > 0 {
		assignee = issue.AssigneeDetails[0].DisplayName
	}

	return pmprovider.Item{
		ID:          issue.ID,
		ExternalID:  fmt.Sprintf("%s#%s", projectRef, issue.ID),
		Title:       issue.Name,
		Description: issue.DescriptionStripped,
		Status:      mapStatus(issue.StateDetail.Group),
		Labels:      labels,
		Priority:    issue.Priority,
		Assignee:    assignee,
	}
}

func (p *Provider) ListItems(ctx context.Context, projectRef string) ([]pmprovider.Item, error) {
	workspace, projectID, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	var allItems []pmprovider.Item
	cursor := ""

	for {
		url := p.issuesURL(workspace, projectID)
		if cursor != "" {
			url += "?cursor=" + neturl.QueryEscape(cursor)
		}

		resp, err := p.doRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("plane: list issues: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("plane: list issues: status %d: %s", resp.StatusCode, string(body))
		}

		var listResp planeListResponseBody
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("plane: decode list response: %w", err)
		}
		_ = resp.Body.Close()

		for i := range listResp.Results {
			allItems = append(allItems, issueToItem(&listResp.Results[i], projectRef))
		}

		if !listResp.NextPageResults || listResp.NextCursor == "" {
			break
		}
		cursor = listResp.NextCursor
	}

	if allItems == nil {
		allItems = []pmprovider.Item{}
	}
	return allItems, nil
}

func (p *Provider) GetItem(ctx context.Context, projectRef, itemID string) (*pmprovider.Item, error) {
	workspace, projectID, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s/", p.issuesURL(workspace, projectID), itemID)

	resp, err := p.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("plane: get issue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plane: get issue %q: status %d: %s", itemID, resp.StatusCode, string(body))
	}

	var issue planeIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("plane: decode issue response: %w", err)
	}

	item := issueToItem(&issue, projectRef)
	return &item, nil
}

// planeCreatePayload is the JSON body sent to create/update a Plane issue.
type planeCreatePayload struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty"`
}

func (p *Provider) CreateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	workspace, projectID, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	payload := planeCreatePayload{
		Name:        item.Title,
		Description: item.Description,
		Priority:    item.Priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("plane: marshal create payload: %w", err)
	}

	url := p.issuesURL(workspace, projectID)
	resp, err := p.doRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("plane: create issue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plane: create issue: status %d: %s", resp.StatusCode, string(respBody))
	}

	var created planeIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("plane: decode created issue: %w", err)
	}

	result := issueToItem(&created, projectRef)
	return &result, nil
}

func (p *Provider) UpdateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	if item.ID == "" {
		return nil, fmt.Errorf("plane: item ID is required for update")
	}

	workspace, projectID, err := parseProjectRef(projectRef)
	if err != nil {
		return nil, err
	}

	payload := planeCreatePayload{
		Name:        item.Title,
		Description: item.Description,
		Priority:    item.Priority,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("plane: marshal update payload: %w", err)
	}

	url := fmt.Sprintf("%s%s/", p.issuesURL(workspace, projectID), item.ID)
	resp, err := p.doRequest(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("plane: update issue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plane: update issue %q: status %d: %s", item.ID, resp.StatusCode, string(respBody))
	}

	var updated planeIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("plane: decode updated issue: %w", err)
	}

	result := issueToItem(&updated, projectRef)
	return &result, nil
}
