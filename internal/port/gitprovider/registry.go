package gitprovider

import (
	"fmt"
	"sync"
)

// Factory is a constructor function that creates a new Provider instance.
type Factory func(config map[string]string) (Provider, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register makes a git provider factory available by name.
// It is typically called from an init() function in the adapter package.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("gitprovider: duplicate registration for %q", name))
	}
	factories[name] = factory
}

// New creates a new Provider by name using the registered factory.
func New(name string, config map[string]string) (Provider, error) {
	mu.RLock()
	factory, ok := factories[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("gitprovider: unknown provider %q", name)
	}
	return factory(config)
}

// Available returns the names of all registered providers.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
