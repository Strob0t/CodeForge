package secrets

import "strings"

const redactedUserinfo = "[REDACTED]"

// RedactURL replaces the userinfo (user, password or token) of every URL in s
// with [REDACTED] and keeps the scheme and everything after the userinfo, so
// URLs and DSNs can be logged.
//
// After each "scheme://" the userinfo runs to the last "@" before the next
// whitespace, quote, bracket or "://". That covers passwords containing "@",
// ":" or an unencoded "/" (base64), URLs inside quotes, parentheses or
// brackets, and comma-separated server lists, whatever follows the host. It
// errs on the side of redacting: an "@" later in the path or query of a URL
// is treated as the end of its userinfo. Unlike url.URL.Redacted it also hides
// a token-only userinfo. The values of credential query parameters
// (IsCredentialName: key, api_key, token, X-Amz-Signature, ...) are replaced
// with [REDACTED] as well. It runs in linear time.
func RedactURL(s string) string {
	return RedactURLWith(s, redactedUserinfo)
}

// RedactURLWith is RedactURL with marker in place of [REDACTED] (the MCP API
// shows "***", which a URL parser accepts in userinfo and query values). A
// value that already is marker stays, so redacting twice changes nothing.
func RedactURLWith(s, marker string) string {
	// Query values first: a redacted userinfo ("[REDACTED]@") would end the
	// URL for the query scan at its bracket.
	return redactUserinfo(redactQueryCredentials(s, marker), marker)
}

// redactUserinfo replaces the userinfo of every URL in s (see RedactURL).
func redactUserinfo(s, marker string) string {
	var b strings.Builder
	copied := 0 // s[:copied] is already in b
	pos := 0
	for {
		k := strings.Index(s[pos:], "://")
		if k < 0 {
			break
		}
		sep := pos + k
		start := sep + 3
		pos = start
		if sep == 0 || !isSchemeByte(s[sep-1]) {
			continue
		}
		end := start + authorityEnd(s[start:])
		pos = end
		at := strings.LastIndexByte(s[start:end], '@')
		if at < 0 {
			continue
		}
		b.WriteString(s[copied:start])
		b.WriteString(marker)
		copied = start + at
	}
	if copied == 0 {
		return s
	}
	b.WriteString(s[copied:])
	return b.String()
}

func isSchemeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// authorityEnd returns the length of the prefix of s that can hold
// "userinfo@host": up to the next whitespace, quote, bracket or "://".
func authorityEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v', '"', '\'', '`', '(', ')', '[', ']', '{', '}', '<', '>':
			return i
		case ':':
			if strings.HasPrefix(s[i:], "://") {
				return i
			}
		}
	}
	return len(s)
}

// redactQueryCredentials replaces the values of the credential query
// parameters of every URL in s with marker; an empty or already redacted
// value stays.
func redactQueryCredentials(s, marker string) string {
	var b strings.Builder
	copied := 0 // s[:copied] is already in b
	pos := 0
	for {
		k := strings.Index(s[pos:], "://")
		if k < 0 {
			break
		}
		sep := pos + k
		start := sep + 3
		pos = start
		if sep == 0 || !isSchemeByte(s[sep-1]) {
			continue
		}
		end := start + authorityEnd(s[start:])
		pos = end
		q := strings.IndexByte(s[start:end], '?')
		if q < 0 {
			continue
		}
		queryEnd := end
		if hash := strings.IndexByte(s[start+q:end], '#'); hash >= 0 {
			queryEnd = start + q + hash
		}
		for i := start + q + 1; i < queryEnd; {
			paramEnd := queryEnd
			if amp := strings.IndexByte(s[i:queryEnd], '&'); amp >= 0 {
				paramEnd = i + amp
			}
			if eq := strings.IndexByte(s[i:paramEnd], '='); eq >= 0 {
				valueStart := i + eq + 1
				value := s[valueStart:paramEnd]
				if value != "" && value != marker && IsCredentialName(s[i:i+eq]) {
					b.WriteString(s[copied:valueStart])
					b.WriteString(marker)
					copied = paramEnd
				}
			}
			i = paramEnd + 1
		}
	}
	if copied == 0 {
		return s
	}
	b.WriteString(s[copied:])
	return b.String()
}
