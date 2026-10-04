package config

import (
	"fmt"
	"net/url"
)

// checkHTTPBaseURL checks that raw is an absolute http(s) URL without user
// info, query or fragment (a base URL other URLs are built on).
func checkHTTPBaseURL(key, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return fmt.Errorf("%s: %q must be an absolute http(s) URL without user info, query or fragment", key, raw)
	}
	return nil
}
