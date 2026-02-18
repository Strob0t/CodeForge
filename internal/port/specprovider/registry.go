package specprovider

import (
	"fmt"
	"sort"
	"sync"
)

// Factory creates a new Provider from config.
type Factory func(config map[string]string) (Provider, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register adds a spec provider factory under the given name.
// It panics on duplicate registration.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("specprovider: duplicate registration %q", name))
	}
	factories[name] = factory
}

// New creates a provider by name using the registered factory.
func New(name string, config map[string]string) (Provider, error) {
	mu.RLock()
	defer mu.RUnlock()

	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("specprovider: unknown provider %q", name)
	}
	return factory(config)
}

// Available returns the sorted list of registered provider names.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
