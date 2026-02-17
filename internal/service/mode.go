package service

import (
	"fmt"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/mode"
)

// ModeService manages agent modes (built-in + custom).
type ModeService struct {
	mu    sync.RWMutex
	modes map[string]mode.Mode
}

// NewModeService creates a ModeService pre-loaded with built-in modes.
func NewModeService() *ModeService {
	s := &ModeService{modes: make(map[string]mode.Mode)}
	for _, m := range mode.BuiltinModes() {
		s.modes[m.ID] = m
	}
	return s
}

// List returns all modes (built-in + custom).
func (s *ModeService) List() []mode.Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]mode.Mode, 0, len(s.modes))
	for _, m := range s.modes {
		result = append(result, m)
	}
	return result
}

// Get returns a mode by ID.
func (s *ModeService) Get(id string) (*mode.Mode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.modes[id]
	if !ok {
		return nil, fmt.Errorf("mode %q not found", id)
	}
	return &m, nil
}

// Register adds a custom mode. Built-in modes cannot be overwritten.
func (s *ModeService) Register(m *mode.Mode) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validate mode: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.modes[m.ID]; ok && existing.Builtin {
		return fmt.Errorf("cannot overwrite built-in mode %q", m.ID)
	}
	s.modes[m.ID] = *m
	return nil
}
