package http

import (
	"net/http"
)

// StartGitHubOAuth handles GET /api/v1/auth/github.
// It generates the GitHub OAuth authorization URL and redirects the user.
func (h *Handlers) StartGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	if h.GitHubOAuth == nil {
		writeError(w, http.StatusNotImplemented, "GitHub OAuth is not configured")
		return
	}

	authURL, err := h.GitHubOAuth.AuthorizeURL(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate authorization URL")
		return
	}

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GitHubOAuthCallback handles GET /api/v1/auth/github/callback.
// It exchanges the authorization code for an access token and creates a VCS account.
func (h *Handlers) GitHubOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.GitHubOAuth == nil {
		writeError(w, http.StatusNotImplemented, "GitHub OAuth is not configured")
		return
	}

	// Check for error from GitHub (e.g. user denied access) before parsing code/state.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		writeError(w, http.StatusBadRequest, "GitHub authorization failed: "+errParam)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state parameter")
		return
	}

	account, err := h.GitHubOAuth.HandleCallback(r.Context(), code, state)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Clear the encrypted token from the response.
	account.EncryptedToken = nil

	writeJSON(w, http.StatusOK, account)
}
