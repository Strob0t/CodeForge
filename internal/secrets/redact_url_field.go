package secrets

import (
	"net/url"
	"strings"
)

// RedactURLField redacts one URL value, such as the url of an MCP server
// that is shown to users (KI-97 security review), as net/url reads it:
//   - the fragment starts at the first "#", the query at the first "?" before
//     it, and the authority after "scheme://" (from the start when there is
//     none) ends at the first "/", so an "@" in the path, query or fragment
//     never changes the host that is shown;
//   - a userinfo (everything before the last "@" of the authority, a
//     token-only user too) becomes marker;
//   - in the query, and in a fragment of name=value pairs, the non-empty
//     value of every credential parameter (IsCredentialName on the decoded
//     name) becomes marker.
//
// Everything else stays byte-identical, and a value that already is marker
// stays, so redacting twice gives the same string. Unlike RedactURL (free
// text, for logs) it does not stop at quotes, brackets or spaces, which
// net/url accepts in userinfo and query values.
func RedactURLField(raw, marker string) string {
	parts := splitURLField(raw)
	if parts.at >= 0 {
		parts.authority = marker + parts.authority[parts.at:]
	}
	if parts.hasQuery {
		parts.query = redactCredentialPairs(parts.query, marker)
	}
	if parts.hasFragment {
		parts.fragment = redactCredentialPairs(parts.fragment, marker)
	}
	return parts.String()
}

// URLFieldSecrets returns the secret parts of one URL value, as written and
// decoded: the userinfo, its password (or the user when it is the only
// part: a token), and the values of credential parameters in the query and
// fragment. Text that may quote the URL (an error) is scrubbed with them.
func URLFieldSecrets(raw string) []string {
	parts := splitURLField(raw)
	var found []string
	add := func(s string) {
		found = append(found, s)
		if decoded, err := url.PathUnescape(s); err == nil && decoded != s {
			found = append(found, decoded)
		}
	}
	if parts.at > 0 {
		userinfo := parts.authority[:parts.at]
		add(userinfo)
		if user, password, ok := strings.Cut(userinfo, ":"); ok {
			add(password)
		} else {
			add(user)
		}
	}
	for _, part := range []string{parts.query, parts.fragment} {
		for _, pair := range strings.Split(part, "&") {
			if name, value, ok := strings.Cut(pair, "="); ok && value != "" && isCredentialParam(name) {
				add(value)
				if decoded, err := url.QueryUnescape(value); err == nil && decoded != value {
					found = append(found, decoded)
				}
			}
		}
	}
	return found
}

// urlField is a URL value split as net/url splits it; String joins the parts.
type urlField struct {
	prefix                string // up to and including "://" (empty without a scheme)
	authority             string
	rest                  string // path (and opaque parts) up to the query
	query, fragment       string
	hasQuery, hasFragment bool
	at                    int // index of the last "@" in authority, -1 for none
}

func splitURLField(raw string) urlField {
	var f urlField
	rest, fragment, hasFragment := strings.Cut(raw, "#")
	rest, query, hasQuery := strings.Cut(rest, "?")
	f.query, f.hasQuery, f.fragment, f.hasFragment = query, hasQuery, fragment, hasFragment
	if i := strings.Index(rest, "://"); i >= 0 && isScheme(rest[:i]) {
		f.prefix, rest = rest[:i+len("://")], rest[i+len("://"):]
	}
	f.authority, f.rest = rest, ""
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		f.authority, f.rest = rest[:slash], rest[slash:]
	}
	f.at = strings.LastIndexByte(f.authority, '@')
	return f
}

func (f *urlField) String() string {
	var b strings.Builder
	b.WriteString(f.prefix)
	b.WriteString(f.authority)
	b.WriteString(f.rest)
	if f.hasQuery {
		b.WriteByte('?')
		b.WriteString(f.query)
	}
	if f.hasFragment {
		b.WriteByte('#')
		b.WriteString(f.fragment)
	}
	return b.String()
}

func isScheme(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isSchemeByte(s[i]) {
			return false
		}
	}
	return true
}

// redactCredentialPairs replaces the non-empty values of the credential
// parameters of an "&"-separated list of name=value pairs with marker.
func redactCredentialPairs(pairs, marker string) string {
	items := strings.Split(pairs, "&")
	for i, item := range items {
		if name, value, ok := strings.Cut(item, "="); ok && value != "" && value != marker && isCredentialParam(name) {
			items[i] = name + "=" + marker
		}
	}
	return strings.Join(items, "&")
}

func isCredentialParam(name string) bool {
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}
	return IsCredentialName(name)
}
