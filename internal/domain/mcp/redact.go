package mcp

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// RedactedValue stands for a stored secret of an MCP server: a set env
// variable or header value (KI-71 review), the password of the url and the
// value of an argument that carries a credential (KI-97). The API shows
// where one is set, never its value, and a client that sends it back keeps
// the stored value.
const RedactedValue = "***"

// Redacted returns a copy of s whose secrets are RedactedValue: set env and
// header values, the url's password (a user without a password stays) and
// credential argument values (see credentialArg). Empty values stay empty;
// keys, flags and the other fields stay.
func (s *ServerDef) Redacted() ServerDef {
	out := *s
	out.URL = redactURLPassword(s.URL)
	out.Args = slices.Clone(s.Args)
	for i := range out.Args {
		if prefix, value, ok := credentialArg(s.Args, i); ok && value != "" {
			out.Args[i] = prefix + RedactedValue
		}
	}
	out.Env = redactValues(s.Env)
	out.Headers = redactValues(s.Headers)
	return out
}

// KeepRedacted replaces every RedactedValue of s (as Redacted writes them)
// with the stored value: env and header values of the same key, the url's
// password, and an argument value at the same position with the same flag.
// The stored values go only where they were stored for: with anything
// redacted, transport, url, command and arguments must equal stored once the
// values are restored. stored is nil for a new server. A RedactedValue that
// stands for nothing stored, or another destination, is a
// domain.ErrValidation error, and s is then left as it was.
func (s *ServerDef) KeepRedacted(stored *ServerDef) error {
	if !s.HasRedacted() {
		return nil
	}
	if stored == nil {
		return fmt.Errorf("%w: %q stands for a stored secret, but nothing is stored for this server; enter the values", domain.ErrValidation, RedactedValue)
	}
	url, err := keepURLPassword(s.URL, stored.URL)
	if err != nil {
		return err
	}
	args, err := keepArgs(s.Args, stored.Args)
	if err != nil {
		return err
	}
	if s.Transport != stored.Transport || url != stored.URL || s.Command != stored.Command || !slices.Equal(args, stored.Args) {
		return fmt.Errorf("%w: stored secrets (env, headers, the url's password, credential arguments) are kept only for the same transport, url, command and arguments; enter them again", domain.ErrValidation)
	}
	env, err := keepValues("env", s.Env, stored.Env)
	if err != nil {
		return err
	}
	headers, err := keepValues("header", s.Headers, stored.Headers)
	if err != nil {
		return err
	}
	s.URL, s.Args, s.Env, s.Headers = url, args, env, headers
	return nil
}

// HasRedacted reports whether s carries a RedactedValue where Redacted
// writes one.
func (s *ServerDef) HasRedacted() bool {
	for _, values := range []map[string]string{s.Env, s.Headers} {
		for _, v := range values {
			if v == RedactedValue {
				return true
			}
		}
	}
	if _, password, _, ok := urlPassword(s.URL); ok && password == RedactedValue {
		return true
	}
	for i := range s.Args {
		if _, value, ok := credentialArg(s.Args, i); ok && value == RedactedValue {
			return true
		}
	}
	return false
}

func redactValues(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if v != "" {
			v = RedactedValue
		}
		out[k] = v
	}
	return out
}

// keepValues returns values with every RedactedValue replaced by the value
// of the same key in stored.
func keepValues(kind string, values, stored map[string]string) (map[string]string, error) {
	keys := slices.Collect(maps.Keys(values))
	sort.Strings(keys)
	out := maps.Clone(values)
	for _, k := range keys {
		if values[k] != RedactedValue {
			continue
		}
		old, ok := stored[k]
		if !ok || old == "" {
			return nil, fmt.Errorf("%w: %s %q is %q, but no value is stored for it", domain.ErrValidation, kind, k, RedactedValue)
		}
		out[k] = old
	}
	return out, nil
}

// urlPassword splits rawURL around the password of its userinfo, as written
// (percent-encoded): rawURL == before + password + after. ok is false when
// the url has no password or an empty one. The authority ends at the first
// "/", "?" or "#", the userinfo at its last "@" and the user at the first
// ":", as url.Parse reads them; a url without "://" is read from its start.
func urlPassword(rawURL string) (before, password, after string, ok bool) {
	start := 0
	if i := strings.Index(rawURL, "://"); i >= 0 {
		start = i + len("://")
	}
	end := len(rawURL)
	if i := strings.IndexAny(rawURL[start:], "/?#"); i >= 0 {
		end = start + i
	}
	at := strings.LastIndexByte(rawURL[start:end], '@')
	if at < 0 {
		return "", "", "", false
	}
	colon := strings.IndexByte(rawURL[start:start+at], ':')
	if colon < 0 || colon == at-1 {
		return "", "", "", false
	}
	return rawURL[:start+colon+1], rawURL[start+colon+1 : start+at], rawURL[start+at:], true
}

func redactURLPassword(rawURL string) string {
	before, _, after, ok := urlPassword(rawURL)
	if !ok {
		return rawURL
	}
	return before + RedactedValue + after
}

// keepURLPassword returns rawURL with a RedactedValue password replaced by
// the password of storedURL (whatever else differs is refused afterwards).
func keepURLPassword(rawURL, storedURL string) (string, error) {
	before, password, after, ok := urlPassword(rawURL)
	if !ok || password != RedactedValue {
		return rawURL, nil
	}
	_, storedPassword, _, stored := urlPassword(storedURL)
	if !stored {
		return "", fmt.Errorf("%w: the url's password is %q, but no password is stored for it", domain.ErrValidation, RedactedValue)
	}
	return before + storedPassword + after, nil
}

// credentialArg reports whether args[i] holds the value of a credential
// argument: "--name=value", "-name=value" or "name=value" (prefix is the part
// up to "="), or args[i] follows "--name" or "-name" (prefix is "") - where
// name is a credential name (secrets.IsCredentialName: token, api-key,
// password, githubToken, ...).
func credentialArg(args []string, i int) (prefix, value string, ok bool) {
	arg := args[i]
	if eq := strings.IndexByte(arg, '='); eq > 0 {
		if name := strings.TrimLeft(arg[:eq], "-"); name != "" && secrets.IsCredentialName(name) {
			return arg[:eq+1], arg[eq+1:], true
		}
	}
	if i > 0 {
		if flag := args[i-1]; strings.HasPrefix(flag, "-") && !strings.Contains(flag, "=") {
			if name := strings.TrimLeft(flag, "-"); name != "" && secrets.IsCredentialName(name) {
				return "", arg, true
			}
		}
	}
	return "", "", false
}

// keepArgs returns args with every RedactedValue credential value replaced by
// the stored argument at the same position, when its flag is the same.
func keepArgs(args, stored []string) ([]string, error) {
	out := slices.Clone(args)
	for i := range args {
		prefix, value, ok := credentialArg(args, i)
		if !ok || value != RedactedValue {
			continue
		}
		sameFlag := i < len(stored)
		if sameFlag && prefix == "" {
			sameFlag = stored[i-1] == args[i-1]
		} else if sameFlag {
			sameFlag = strings.HasPrefix(stored[i], prefix)
		}
		if !sameFlag || stored[i] == prefix {
			return nil, fmt.Errorf("%w: argument %d is %q, but no value is stored for it with the same flag", domain.ErrValidation, i+1, RedactedValue)
		}
		out[i] = stored[i]
	}
	return out, nil
}
