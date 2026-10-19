// Package githubapi is the GitHub REST API client of the GitHub adapters:
// the github-issues PM provider and the pull requests of PR delivery
// (KI-117; the Go Core runs no gh CLI).
//
// Requests carry the token as a bearer token, connect through an outbound
// policy (never through a proxy, never to link-local or metadata addresses)
// and follow redirects and next-page links only within the API's origin, so
// the token never reaches another host. Errors name the method, the path and
// the status; they never carry the token or the URL of the API, and only
// GitHub's own short error message.
package githubapi

import (
	"bytes"
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
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

// DefaultBaseURL is the API of github.com.
const DefaultBaseURL = "https://api.github.com"

// requestTimeout bounds one request, its redirects included.
const requestTimeout = 30 * time.Second

// maxResponseBytes bounds an answer (a page of 100 issues is far smaller).
const maxResponseBytes = 10 << 20

// maxMessageRunes bounds the part of GitHub's error message an error quotes.
const maxMessageRunes = 300

// apiVersion is the REST API version the requests ask for.
const apiVersion = "2022-11-28"

// NewHTTPClient returns the HTTP client of the GitHub adapters: it connects
// only to addresses policy allows and follows redirects only within the
// origin.
func NewHTTPClient(policy *netutil.OutboundPolicy) *http.Client {
	return &http.Client{Timeout: requestTimeout, Transport: policy.Transport(), CheckRedirect: netutil.SameOriginRedirect}
}

// PublicHTTPClient returns a client that connects to public addresses only.
func PublicHTTPClient() *http.Client {
	policy, _ := netutil.NewOutboundPolicy(nil) // only allowlist entries can be invalid
	return NewHTTPClient(policy)
}

// Client sends requests to one GitHub API with one token ("" sends none:
// public repositories can be read anonymously).
type Client struct {
	base       *url.URL
	token      string
	httpClient *http.Client
}

// NewClient returns a client of the API at baseURL ("" for github.com).
func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	base, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil ||
		base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("%w: the GitHub API base URL must be an absolute http(s) URL without credentials, query or fragment", domain.ErrValidation)
	}
	return &Client{base: base, token: token, httpClient: httpClient}, nil
}

// BaseURL returns the API's base URL.
func (c *Client) BaseURL() string { return c.base.String() }

// Page is a 2xx answer: its body and the path of the next page ("" for the
// last one).
type Page struct {
	Body []byte
	Next string
}

// Error is a non-2xx answer of the API. It wraps domain.ErrNotFound for 404
// and domain.ErrValidation for 401, 403 and 422: a reference, a token or a
// field to correct, not a fault of CodeForge.
type Error struct {
	Method  string
	Path    string
	Status  int
	Message string // GitHub's message, shortened
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("GitHub API %s %s answered %d %s", e.Method, e.Path, e.Status, http.StatusText(e.Status))
	switch e.Status {
	case http.StatusUnauthorized:
		msg += ": the token is missing, invalid or expired"
	case http.StatusForbidden:
		msg += ": the token lacks a permission, or the rate limit is exhausted"
	}
	if e.Message != "" {
		msg += " (" + e.Message + ")"
	}
	return msg
}

func (e *Error) Unwrap() error {
	switch e.Status {
	case http.StatusNotFound:
		return domain.ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity:
		return domain.ErrValidation
	default:
		return nil
	}
}

// Do sends method to path (below the base URL, with its query) with body as
// JSON (nil for none) and returns the answer of a 2xx status.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*Page, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("GitHub API path %q must start with /", path)
	}
	target, err := url.Parse(c.base.String() + path)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid GitHub API path %q", domain.ErrValidation, path)
	}
	return c.do(ctx, method, target, body)
}

// GetNext fetches the next page a Page named.
func (c *Client) GetNext(ctx context.Context, next string) (*Page, error) {
	target, err := url.Parse(next)
	if err != nil {
		return nil, errors.New("GitHub API: invalid next page link")
	}
	return c.do(ctx, http.MethodGet, target, nil)
}

func (c *Client) do(ctx context.Context, method string, target *url.URL, body []byte) (*Page, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("GitHub API %s %s: invalid request", method, target.Path)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", "CodeForge")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // the outbound policy checks every address dialled
	if err != nil {
		return nil, c.requestError(ctx, method, target.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, c.requestError(ctx, method, target.Path, err)
	}
	if len(respBody) > maxResponseBytes {
		return nil, fmt.Errorf("GitHub API %s %s: the answer is larger than %d MiB", method, target.Path, maxResponseBytes>>20)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &Error{Method: method, Path: target.Path, Status: resp.StatusCode, Message: errorMessage(respBody)}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			apiErr.Message = "a redirect is followed only within the API's origin"
		}
		return nil, apiErr
	}
	return &Page{Body: respBody, Next: c.nextLink(resp.Header.Get("Link"))}, nil
}

// nextLink returns the rel="next" URL of a Link header when it lies on the
// API's origin (the token goes with it), else "".
func (c *Client) nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		target = strings.TrimSpace(target)
		if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
			return ""
		}
		next, err := url.Parse(target[1 : len(target)-1])
		if err != nil || next.User != nil || !strings.EqualFold(next.Scheme, c.base.Scheme) || !strings.EqualFold(next.Host, c.base.Host) {
			return ""
		}
		return next.String()
	}
	return ""
}

// errorMessage returns GitHub's "message" and the messages of its "errors",
// shortened; "" when the body is not GitHub's error JSON.
func errorMessage(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	parts := []string{}
	if parsed.Message != "" {
		parts = append(parts, parsed.Message)
	}
	for _, e := range parsed.Errors {
		switch {
		case e.Message != "":
			parts = append(parts, e.Message)
		case e.Field != "" || e.Code != "":
			parts = append(parts, strings.TrimSpace(e.Field+" "+e.Code))
		}
	}
	msg := strings.ToValidUTF8(strings.Join(parts, "; "), "")
	if runes := []rune(msg); len(runes) > maxMessageRunes {
		msg = string(runes[:maxMessageRunes]) + "..."
	}
	return msg
}

// requestError describes a request that failed before an answer by its
// method, its path and what failed; Go's error quotes the whole URL. A
// refused address is passed on (netutil.ErrAddressRefused), with the
// setting that opens it when the operator can.
func (c *Client) requestError(ctx context.Context, method, path string, err error) error {
	var refused *netutil.RefusedAddressError
	if errors.As(err, &refused) {
		if refused.Allowable() {
			// A GitHub Enterprise Server on a private network (KI-166).
			return fmt.Errorf("GitHub API %s %s: %w; only the platform operator can allow it (pm.allowed_private_hosts)", method, path, refused)
		}
		return fmt.Errorf("GitHub API %s %s: %w", method, path, refused)
	}
	cause := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		cause = urlErr.Err
	}
	slog.WarnContext(ctx, "github api request failed", "method", method, "host", c.base.Host, "path", path, "error", cause)
	return fmt.Errorf("GitHub API %s %s: %s", method, path, failureKind(err))
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

// List GETs path and the pages after it (Link rel="next", within the API's
// origin), at most maxPages, and returns the elements of every page; more
// reports that pages were left.
func List[T any](ctx context.Context, c *Client, path string, maxPages int) (items []T, more bool, err error) {
	next := ""
	for pages := 0; pages < maxPages; pages++ {
		var page *Page
		if pages == 0 {
			page, err = c.Do(ctx, http.MethodGet, path, nil)
		} else {
			page, err = c.GetNext(ctx, next)
		}
		if err != nil {
			return nil, false, err
		}
		elements, err := Decode[[]T](page.Body)
		if err != nil {
			return nil, false, err
		}
		items = append(items, elements...)
		if page.Next == "" {
			return items, false, nil
		}
		next = page.Next
	}
	return items, true, nil
}

// Decode parses a JSON answer. A JSON error quotes the answer, so only the
// fact is passed on.
func Decode[T any](body []byte) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, errors.New("GitHub API: the answer is not the expected JSON")
	}
	return v, nil
}

// RepoPath returns "/repos/<owner>/<name>" for "owner/name", each part
// path-escaped. It refuses anything else: GitHub owner and repository names
// are letters, digits, '-', '_' and '.', never "." or "..".
func RepoPath(repo string) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || !validName(owner) || !validName(name) {
		return "", fmt.Errorf("%w: invalid repository %q: expected owner/repo", domain.ErrValidation, repo)
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

func validName(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 100 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
