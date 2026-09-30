package http

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
)

// githubOAuthCookie binds a GitHub OAuth flow to the browser that started
// it: the callback must carry the state in this cookie as well as in the
// query (CSRF protection on top of the single-use, tenant-bound state).
const githubOAuthCookie = "codeforge_github_oauth"

const githubOAuthNotConfigured = "GitHub OAuth is not configured: set github.client_id, github.client_secret and " +
	"github.callback_url (GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET or GITHUB_CLIENT_SECRET_FILE, GITHUB_CALLBACK_URL)"

func (h *Handlers) setGitHubOAuthCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     githubOAuthCookie,
		Value:    value,
		Path:     config.GitHubCallbackPath,
		HttpOnly: true,
		Secure:   h.isSecureCookie(r),
		SameSite: http.SameSiteLaxMode, // sent on GitHub's top-level redirect back
		MaxAge:   maxAge,
	})
}

// StartGitHubOAuth handles POST /api/v1/auth/github (authenticated).
// It stores a state for the caller's tenant, binds it to the browser with
// a cookie and returns GitHub's authorization URL for the UI to open.
func (h *Handlers) StartGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	if h.GitHubOAuth == nil {
		writeError(w, http.StatusNotImplemented, githubOAuthNotConfigured)
		return
	}

	authURL, state, err := h.GitHubOAuth.AuthorizeURL(r.Context())
	if err != nil {
		slog.Error("github oauth: start failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate authorization URL")
		return
	}

	h.setGitHubOAuthCookie(w, r, state, int((10 * time.Minute).Seconds()))
	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

// GitHubOAuthCallback handles GET /api/v1/auth/github/callback, GitHub's
// redirect back (no session). It connects the account and sends the browser
// to the settings page with github_oauth=connected, or github_oauth=failed
// and a fixed reason code.
func (h *Handlers) GitHubOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.GitHubOAuth == nil {
		writeError(w, http.StatusNotImplemented, githubOAuthNotConfigured)
		return
	}
	h.setGitHubOAuthCookie(w, r, "", -1)

	back := func(query string) {
		http.Redirect(w, r, h.GitHubOAuth.UIURL("/settings?"+query), http.StatusSeeOther)
	}
	fail := func(reason string) { back("github_oauth=failed&reason=" + reason) }

	q := r.URL.Query()
	if q.Get("error") != "" {
		fail("denied")
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		fail("invalid_request")
		return
	}
	cookie, err := r.Cookie(githubOAuthCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		fail("state_mismatch")
		return
	}

	if _, err := h.GitHubOAuth.HandleCallback(r.Context(), code, state); err != nil {
		slog.Warn("github oauth: callback failed", "error", err)
		if errors.Is(err, domain.ErrNotFound) {
			fail("invalid_state")
			return
		}
		fail("exchange_failed")
		return
	}
	back("github_oauth=connected")
}
