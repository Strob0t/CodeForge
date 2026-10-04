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
// variable or header value (KI-71 review), the userinfo and credential query
// values of the url and the value of an argument that carries a credential
// (KI-97). The API shows
// where one is set, never its value, and a client that sends it back keeps
// the stored value.
const RedactedValue = "***"

// Redacted returns a copy of s whose secrets are RedactedValue: set env and
// header values, the url's userinfo and credential query and fragment
// values (secrets.RedactURLField) and credential argument values (see
// credentialArg). Empty values stay empty;
// keys, flags and the other fields stay.
func (s *ServerDef) Redacted() ServerDef {
	out := *s
	out.URL = secrets.RedactURLField(s.URL, RedactedValue)
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

// KeepRedacted replaces the RedactedValues of s with what they stand for in
// stored (nil for a new server). The url and the argument list are taken
// from stored as a whole when they come back exactly as Redacted shows them;
// a url or argument list that still carries RedactedValue otherwise is
// refused (no value is guessed from positions or flags). Env and header
// values sent as RedactedValue keep the stored value of the same key. Every
// restored value goes only where it was stored for: transport, url, command
// and arguments must then equal stored (another command with the stored
// arguments, or another endpoint with the stored headers, would receive the
// secrets). A violation is a domain.ErrValidation error, and s is then left
// as it was.
func (s *ServerDef) KeepRedacted(stored *ServerDef) error {
	if !s.HasRedacted() {
		return nil
	}
	if stored == nil {
		return fmt.Errorf("%w: %q stands for a stored secret, but nothing is stored for this server; enter the values", domain.ErrValidation, RedactedValue)
	}
	read := stored.Redacted()
	url := s.URL
	if strings.Contains(url, RedactedValue) {
		if url != read.URL {
			return fmt.Errorf("%w: the url carries %q but is not the url as read; enter it with its secrets again", domain.ErrValidation, RedactedValue)
		}
		url = stored.URL
	}
	args := s.Args
	if slices.ContainsFunc(args, isRedactedArg) {
		if !slices.Equal(args, read.Args) {
			return fmt.Errorf("%w: the arguments carry %q but are not the arguments as read; enter them with their secrets again", domain.ErrValidation, RedactedValue)
		}
		args = slices.Clone(stored.Args)
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

// HasRedacted reports whether s carries a RedactedValue: as an env or header
// value, or anywhere in its url or arguments.
func (s *ServerDef) HasRedacted() bool {
	for _, values := range []map[string]string{s.Env, s.Headers} {
		for _, v := range values {
			if v == RedactedValue {
				return true
			}
		}
	}
	return strings.Contains(s.URL, RedactedValue) || slices.ContainsFunc(s.Args, isRedactedArg)
}

func isRedactedArg(arg string) bool { return strings.Contains(arg, RedactedValue) }

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
