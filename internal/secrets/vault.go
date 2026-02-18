// Package secrets provides a thread-safe secret vault with hot reload support.
package secrets

import (
	"fmt"
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
