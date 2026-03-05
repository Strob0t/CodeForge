// Package githubpm implements a pmprovider.Provider for GitHub Issues using the gh CLI.
package githubpm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "github-issues"

// Provider implements pmprovider.Provider for GitHub Issues via the gh CLI.
type Provider struct {
	// execCommand is swappable for testing.
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func newProvider() *Provider {
	return &Provider{execCommand: exec.CommandContext}
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

// ghIssue mirrors the JSON output of `gh issue list/view --json`.
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Assignees []ghUser  `json:"assignees"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghUser struct {
	Login string `json:"login"`
}

func (p *Provider) ListItems(ctx context.Context, projectRef string) ([]pmprovider.Item, error) {
	if err := validateProjectRef(projectRef); err != nil {
		return nil, err
	}

	cmd := p.execCommand(ctx, "gh", "issue", "list",
		"--repo", projectRef,
		"--json", "number,title,body,state,labels,assignees",
		"--limit", "100",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh issue list: %s: %w", stderr.String(), err)
	}

	var issues []ghIssue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}

	items := make([]pmprovider.Item, 0, len(issues))
	for i := range issues {
		items = append(items, issueToItem(&issues[i], projectRef))
	}
	return items, nil
}

func (p *Provider) GetItem(ctx context.Context, projectRef, itemID string) (*pmprovider.Item, error) {
	if err := validateProjectRef(projectRef); err != nil {
		return nil, err
	}

	cmd := p.execCommand(ctx, "gh", "issue", "view", itemID,
		"--repo", projectRef,
		"--json", "number,title,body,state,labels,assignees",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh issue view: %s: %w", stderr.String(), err)
	}

	var issue ghIssue
	if err := json.Unmarshal(stdout.Bytes(), &issue); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}

	item := issueToItem(&issue, projectRef)
	return &item, nil
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
		ID:          fmt.Sprintf("%d", issue.Number),
		ExternalID:  fmt.Sprintf("%s#%d", repo, issue.Number),
		Title:       issue.Title,
		Description: issue.Body,
		Status:      strings.ToLower(issue.State),
		Labels:      labels,
		Assignee:    assignee,
	}
}

// CreateItem creates a GitHub issue via the gh CLI.
func (p *Provider) CreateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	if err := validateProjectRef(projectRef); err != nil {
		return nil, err
	}

	args := []string{"issue", "create", "--repo", projectRef, "--title", item.Title}
	if item.Description != "" {
		args = append(args, "--body", item.Description)
	}
	for _, label := range item.Labels {
		args = append(args, "--label", label)
	}

	cmd := p.execCommand(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh issue create: %s: %w", stderr.String(), err)
	}

	// gh issue create prints the URL; extract the issue number from it.
	url := strings.TrimSpace(stdout.String())
	parts := strings.Split(url, "/")
	issueNum := parts[len(parts)-1]

	created := *item
	created.ID = issueNum
	created.ExternalID = fmt.Sprintf("%s#%s", projectRef, issueNum)
	return &created, nil
}

// UpdateItem updates a GitHub issue via the gh CLI.
func (p *Provider) UpdateItem(ctx context.Context, projectRef string, item *pmprovider.Item) (*pmprovider.Item, error) {
	if err := validateProjectRef(projectRef); err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, fmt.Errorf("item ID is required for update")
	}

	args := []string{"issue", "edit", item.ID, "--repo", projectRef, "--title", item.Title}
	if item.Description != "" {
		args = append(args, "--body", item.Description)
	}

	cmd := p.execCommand(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh issue edit: %s: %w", stderr.String(), err)
	}

	return item, nil
}

func validateProjectRef(ref string) error {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid project ref %q: expected owner/repo", ref)
	}
	return nil
}
