package agentbackend

import (
	"fmt"
	"sync"
)

// Factory is a constructor function that creates a new Backend instance.
type Factory func(config map[string]string) (Backend, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register makes an agent backend factory available by name.
// It is typically called from an init() function in the adapter package.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("agentbackend: duplicate registration for %q", name))
	}
	factories[name] = factory
}

// New creates a new Backend by name using the registered factory.
func New(name string, config map[string]string) (Backend, error) {
	mu.RLock()
	factory, ok := factories[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agentbackend: unknown backend %q", name)
	}
	return factory(config)
}

// Available returns the names of all registered backends.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
