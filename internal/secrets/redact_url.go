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
// a token-only userinfo. It runs in linear time.
func RedactURL(s string) string {
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
		b.WriteString(redactedUserinfo)
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
