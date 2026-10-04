// Package gitlab implements a pmprovider.Provider for GitLab instances using their REST API v4.
package gitlab

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

const providerName = "gitlab"

// requestTimeout bounds one request, its redirects included.
const requestTimeout = 30 * time.Second

// maxResponseBytes bounds an answer (a page of 50 issues is far smaller).
const maxResponseBytes = 10 << 20

// client is the HTTP client of every provider NewProvider creates. The base
// URL is chosen by tenants (the host of a project's repo_url for webhook
// syncs, base_url for a manual sync), so it connects only to the addresses
// its outbound policy allows, checked at the address it dials (DNS
// rebinding, redirects), never through a proxy, and follows a redirect only
// within the origin: the PRIVATE-TOKEN header, which Go forwards to any
// host, stays with its GitLab (KI-85 review). Until SetOutboundPolicy opens
// some, no private or loopback address is allowed.
var client atomic.Pointer[http.Client]

func init() {
	none, _ := netutil.NewOutboundPolicy(nil) // only allowlist entries can be invalid
	SetOutboundPolicy(none)
}

// SetOutboundPolicy makes the providers created from now on connect through
// policy; the Go Core builds it from pm.allowed_private_hosts at startup.
func SetOutboundPolicy(policy *netutil.OutboundPolicy) {
	client.Store(newHTTPClient(policy))
}

func newHTTPClient(policy *netutil.OutboundPolicy) *http.Client {
	return &http.Client{Timeout: requestTimeout, Transport: policy.Transport(), CheckRedirect: netutil.SameOriginRedirect}
}

// Provider implements pmprovider.Provider for GitLab Issues via the REST API v4.
type Provider struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewProvider creates a GitLab provider with the given base URL and private
// token that connects through the outbound policy (SetOutboundPolicy).
func NewProvider(baseURL, token string) *Provider {
	return newProvider(baseURL, token, client.Load())
}

func newProvider(baseURL, token string, httpClient *http.Client) *Provider {
	return &Provider{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
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

	issues, err := decode[[]gitlabIssue](body)
	if err != nil {
		return nil, err
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

	issue, err := decode[gitlabIssue](body)
	if err != nil {
		return nil, err
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
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gitlab marshal create payload: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues", p.baseURL, encodedRef)
	body, err := p.doRequest(ctx, http.MethodPost, reqURL, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitlab create issue: %w", err)
	}

	created, err := decode[gitlabIssue](body)
	if err != nil {
		return nil, err
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
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gitlab marshal update payload: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/v4/projects/%s/issues/%s", p.baseURL, encodedRef, item.ID)
	body, err := p.doRequest(ctx, http.MethodPut, reqURL, strings.NewReader(string(payloadJSON)))
	if err != nil {
		return nil, fmt.Errorf("gitlab update issue: %w", err)
	}

	updated, err := decode[gitlabIssue](body)
	if err != nil {
		return nil, err
	}

	result := issueToItem(&updated, projectRef)
	return &result, nil
}

// doRequest sends a request and returns the body of a successful answer.
// Its errors reach tenants (pm.sync events of webhook syncs), and the host
// may be an internal GitLab the operator allowlisted, so they never
// carry what the server sent, the token or the URL (its userinfo and query
// may hold secrets): only the method, the origin, and the status or the
// kind of failure. The details of a failure are logged.
func (p *Provider) doRequest(ctx context.Context, method, reqURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, errors.New("invalid GitLab request URL")
	}
	if p.token != "" {
		req.Header.Set("PRIVATE-TOKEN", p.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	origin := req.URL.Scheme + "://" + req.URL.Host

	resp, err := p.httpClient.Do(req) //nolint:gosec // tenant-chosen URL: the outbound policy checks every address dialled
	if err != nil {
		return nil, requestError(ctx, method, origin, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The code only: the reason phrase of the status line is the server's.
		status := fmt.Sprintf("%s %s: answered %d %s", method, origin, resp.StatusCode, http.StatusText(resp.StatusCode))
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return nil, errors.New(status + "; a redirect is followed only within the origin")
		}
		if resp.StatusCode == http.StatusNotFound {
			// The project or issue the caller referenced does not exist (or
			// the token cannot see it): a reference to correct, not a fault.
			// The caller gets a plain 404; the operator sees GitLab's answer.
			slog.WarnContext(ctx, "gitlab answered not found", "method", method, "origin", origin, "status", resp.StatusCode)
			return nil, fmt.Errorf("%s: %w", status, domain.ErrNotFound)
		}
		return nil, errors.New(status)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, requestError(ctx, method, origin, err)
	}
	if len(respBody) > maxResponseBytes {
		return nil, fmt.Errorf("%s %s: the answer is larger than %d MiB", method, origin, maxResponseBytes>>20)
	}
	return respBody, nil
}

// requestError describes a request that failed by its method, its origin
// and what failed. Go's error quotes the whole URL and can quote what the
// server sent (a Location header, the start of an answer that is not HTTP);
// the cause is logged instead. A refused address is passed on as it is
// (netutil.ErrAddressRefused): it names the host and the address only.
func requestError(ctx context.Context, method, origin string, err error) error {
	var refused *netutil.RefusedAddressError
	if errors.As(err, &refused) {
		if refused.Allowable() {
			return fmt.Errorf("%s %s: %w; only the platform operator can allow it (pm.allowed_private_hosts)", method, origin, refused)
		}
		return fmt.Errorf("%s %s: %w", method, origin, refused)
	}
	cause := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		cause = urlErr.Err
	}
	slog.WarnContext(ctx, "gitlab request failed", "method", method, "origin", origin, "error", cause)
	return fmt.Errorf("%s %s: %s", method, origin, failureKind(err))
}

// failureKind names what made a request fail, in words that quote nothing
// the server sent.
func failureKind(err error) string {
	var (
		netErr  net.Error
		dnsErr  *net.DNSError
		certErr *tls.CertificateVerificationError
		opErr   *net.OpError
	)
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timed out"
	case errors.As(err, &dnsErr):
		return "the host name does not resolve"
	case errors.As(err, &certErr):
		return "the TLS certificate is not trusted"
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return "the connection failed"
	default:
		return "no valid HTTP answer"
	}
}

// decode parses an answer. A JSON error quotes the answer, so only the
// fact is passed on.
func decode[T any](body []byte) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, errors.New("gitlab parse response: the answer is not GitLab API JSON")
	}
	return v, nil
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
