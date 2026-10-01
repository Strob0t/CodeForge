package mcp

import (
	"fmt"
	"slices"
	"sort"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// RedactedValue stands for a set env variable or header value of an MCP
// server: the API shows which ones are set, never their values (KI-71
// review), and a client that sends it back keeps the stored value.
const RedactedValue = "***"

// Redacted returns a copy of s whose set env and header values are
// RedactedValue; an empty value stays empty, the keys and other fields stay.
func (s *ServerDef) Redacted() ServerDef {
	out := *s
	out.Args = slices.Clone(s.Args)
	out.Env = redactValues(s.Env)
	out.Headers = redactValues(s.Headers)
	return out
}

// KeepRedacted replaces every env and header value of s that is
// RedactedValue with the value of the same key in stored (nil for a new
// server): an update that sends back what it read keeps the secrets. A
// RedactedValue without a stored value is a domain.ErrValidation error.
func (s *ServerDef) KeepRedacted(stored *ServerDef) error {
	var storedEnv, storedHeaders map[string]string
	if stored != nil {
		storedEnv, storedHeaders = stored.Env, stored.Headers
	}
	if err := keepValues("env", s.Env, storedEnv); err != nil {
		return err
	}
	return keepValues("header", s.Headers, storedHeaders)
}

// HasRedacted reports whether an env or header value of s is RedactedValue.
func (s *ServerDef) HasRedacted() bool {
	for _, values := range []map[string]string{s.Env, s.Headers} {
		for _, v := range values {
			if v == RedactedValue {
				return true
			}
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

func keepValues(kind string, values, stored map[string]string) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if values[k] != RedactedValue {
			continue
		}
		old, ok := stored[k]
		if !ok || old == "" {
			return fmt.Errorf("%w: %s %q is %q, but no value is stored for it", domain.ErrValidation, kind, k, RedactedValue)
		}
		values[k] = old
	}
	return nil
}
