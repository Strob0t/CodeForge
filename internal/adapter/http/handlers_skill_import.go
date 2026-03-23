package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/skill"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

type importSkillRequest struct {
	SourceURL string `json:"source_url"`
	ProjectID string `json:"project_id"`
}

type skillRejection struct {
	Error   string   `json:"error"`
	Score   float64  `json:"score"`
	Factors []string `json:"factors"`
}

// ImportSkill handles POST /api/v1/skills/import.
// Fetches content from a URL, checks for injection, and creates the skill.
func (h *Handlers) ImportSkill(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[importSkillRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.SourceURL == "" {
		writeError(w, http.StatusBadRequest, "source_url is required")
		return
	}

	// Fetch content from URL.
	content, contentType, err := fetchURL(r.Context(), req.SourceURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to fetch URL: %v", err))
		return
	}

	// Run quarantine scorer on content to detect prompt injection.
	score, factors := quarantine.ScoreMessage(nil, []byte(content))
	if score > 0.5 {
		writeJSON(w, http.StatusUnprocessableEntity, skillRejection{
			Error:   "skill content rejected due to safety concerns",
			Score:   score,
			Factors: factors,
		})
		return
	}

	formatOrigin := detectFormat(req.SourceURL, contentType)
	name := extractName(req.SourceURL)

	createReq := skill.CreateRequest{
		ProjectID:    req.ProjectID,
		Name:         name,
		Content:      content,
		Description:  fmt.Sprintf("Imported from %s", req.SourceURL),
		Source:       skill.SourceImport,
		SourceURL:    req.SourceURL,
		FormatOrigin: formatOrigin,
		Type:         skill.TypeWorkflow,
	}

	sk, err := h.Skills.Create(r.Context(), &createReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sk)
}

// fetchURL retrieves content from a URL with a 15-second timeout and 1 MB size limit.
func fetchURL(ctx context.Context, rawURL string) (content, contentType string, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("building request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second, Transport: netutil.SafeTransport()}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return "", "", fmt.Errorf("reading response: %w", err)
	}

	return string(body), resp.Header.Get("Content-Type"), nil
}

// detectFormat infers the skill format from URL extension and content type.
func detectFormat(url, contentType string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"):
		return "codeforge"
	case strings.HasSuffix(lower, ".mdc") || strings.Contains(lower, ".cursorrules"):
		return "cursor"
	case strings.HasSuffix(lower, ".md"):
		if strings.Contains(contentType, "yaml") {
			return "claude"
		}
		return "markdown"
	default:
		return "markdown"
	}
}

// extractName derives a skill name from the last path segment of a URL.
func extractName(url string) string {
	idx := strings.LastIndex(url, "/")
	name := url
	if idx >= 0 {
		name = url[idx+1:]
	}
	// Strip query string if present.
	if q := strings.IndexByte(name, '?'); q > 0 {
		name = name[:q]
	}
	// Remove file extension.
	if dot := strings.LastIndex(name, "."); dot > 0 {
		name = name[:dot]
	}
	if name == "" {
		name = "imported-skill"
	}
	return name
}
