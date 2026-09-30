package secrets

import "regexp"

// urlUserinfo matches "scheme://userinfo@host[:port]" followed by the end of
// the authority. The userinfo is the shortest run up to an "@" that is
// followed by a host, so passwords that contain "@", ":" or even an
// unencoded "/" (base64) are covered, and each URL of a comma-separated server
// list is matched on its own. It errs on the side of redacting: an "@" in a
// URL path followed by a host-like segment is treated as userinfo too.
var urlUserinfo = regexp.MustCompile(
	`([A-Za-z][A-Za-z0-9+.-]*://)[^\s?#]*?@([A-Za-z0-9.-]+|\[[0-9A-Fa-f:.]+\])((?::[0-9]+)?(?:[/?#,\s]|$))`,
)

// RedactURL replaces the userinfo (user, password or token) of every URL in s
// with [REDACTED] and keeps scheme, host, port, path and query, so URLs and
// DSNs can be logged. Unlike url.URL.Redacted it also hides a token-only
// userinfo and works on text that is not a single well-formed URL.
func RedactURL(s string) string {
	return urlUserinfo.ReplaceAllString(s, "${1}[REDACTED]@${2}${3}")
}
