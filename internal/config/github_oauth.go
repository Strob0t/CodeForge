package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

// GitHubCallbackPath is the path of the GitHub OAuth web flow's callback.
const GitHubCallbackPath = "/api/v1/auth/github/callback"

// validateGitHubWebFlow checks the GitHub OAuth web flow settings: all three
// or none (client_id alone is the device flow), and a callback URL that
// points at this service's callback.
func validateGitHubWebFlow(g *GitHub) error {
	if g.ClientSecret == "" && g.CallbackURL == "" {
		return nil
	}
	switch {
	case g.ClientID == "":
		return errors.New("github.client_id is required for the GitHub OAuth web flow (github.client_secret / callback_url are set)")
	case g.ClientSecret == "":
		return errors.New("github.client_secret is required for the GitHub OAuth web flow (github.callback_url is set)")
	case g.CallbackURL == "":
		return errors.New("github.callback_url is required for the GitHub OAuth web flow (github.client_secret is set)")
	}
	if err := checkGitHubCallbackURL(g.CallbackURL); err != nil {
		return fmt.Errorf("github.callback_url: %w", err)
	}
	return nil
}

func checkGitHubCallbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("%q must be an absolute URL", raw)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("%q must use https (http only on localhost / loopback)", raw)
	}
	if u.User != nil {
		return fmt.Errorf("%q must not contain user info", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return fmt.Errorf("%q must not have a query or fragment", raw)
	}
	if u.Path != GitHubCallbackPath {
		return fmt.Errorf("%q must have the path %s", raw, GitHubCallbackPath)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
