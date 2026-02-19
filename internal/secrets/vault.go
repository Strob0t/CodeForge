// Package secrets provides a thread-safe secret vault with hot reload support
// and redaction utilities to prevent accidental secret leakage in logs.
package secrets

import (
	"fmt"
	"strings"
	"sync"
)

// Loader retrieves secrets from a source (env vars, file, remote vault, etc.).
type Loader func() (map[string]string, error)

// Vault holds secret values in memory and supports atomic reloading.
type Vault struct {
	mu     sync.RWMutex
	values map[string]string
	loader Loader
}

// NewVault creates a Vault, calling the loader once to populate initial values.
func NewVault(loader Loader) (*Vault, error) {
	vals, err := loader()
	if err != nil {
		return nil, fmt.Errorf("initial secret load: %w", err)
	}
	return &Vault{
		values: vals,
		loader: loader,
	}, nil
}

// Get returns the secret for key, or an empty string if not found.
func (v *Vault) Get(key string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.values[key]
}

// Redacted returns a masked version of the secret for safe use in logs.
// Returns "****" if the key exists, or "" if not found.
func (v *Vault) Redacted(key string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	val, ok := v.values[key]
	if !ok || val == "" {
		return ""
	}
	return maskValue(val)
}

// RedactString replaces any occurrences of stored secret values in the given
// string with masked versions. Use this to sanitize log output, error messages,
// or LLM prompts that might accidentally contain secrets.
func (v *Vault) RedactString(s string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, val := range v.values {
		if val == "" || len(val) < 4 {
			continue
		}
		if strings.Contains(s, val) {
			s = strings.ReplaceAll(s, val, maskValue(val))
		}
	}
	return s
}

// Keys returns the list of secret key names (not values) currently stored.
func (v *Vault) Keys() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	keys := make([]string, 0, len(v.values))
	for k := range v.values {
		keys = append(keys, k)
	}
	return keys
}

// Reload calls the loader and swaps in the new values atomically.
// If the loader returns an error, existing values are preserved.
func (v *Vault) Reload() error {
	newVals, err := v.loader()
	if err != nil {
		return fmt.Errorf("reload secrets: %w", err)
	}
	v.mu.Lock()
	v.values = newVals
	v.mu.Unlock()
	return nil
}

// maskValue returns a masked representation of a secret value.
// Shows the first 2 characters and masks the rest with asterisks,
// capped at a fixed visible length to avoid leaking length information.
func maskValue(val string) string {
	if len(val) <= 4 {
		return "****"
	}
	return val[:2] + "****"
}
