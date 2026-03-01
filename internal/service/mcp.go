package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// MCPService manages MCP server definitions with thread-safe access.
// Definitions can be loaded from YAML files, registered programmatically,
// or persisted to the database via the SetStore method.
type MCPService struct {
	mu         sync.RWMutex
	servers    map[string]mcp.ServerDef
	serversDir string
	db         database.Store
	limits     *config.Limits
}

// NewMCPService creates an MCPService. If cfg.ServersDir is set, definitions
// are loaded from that directory on creation.
func NewMCPService(cfg *config.MCP, limits *config.Limits) *MCPService {
	s := &MCPService{
		servers:    make(map[string]mcp.ServerDef),
		serversDir: cfg.ServersDir,
		limits:     limits,
	}

	if cfg.ServersDir != "" {
		if err := s.LoadFromDirectory(cfg.ServersDir); err != nil {
			slog.Warn("failed to load MCP server definitions", "dir", cfg.ServersDir, "error", err)
		}
	}

	return s
}

// List returns all registered server definitions sorted by ID.
func (s *MCPService) List() []mcp.ServerDef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	defs := make([]mcp.ServerDef, 0, len(s.servers))
	for _, d := range s.servers { //nolint:gocritic // rangeValCopy: map iteration requires value copy
		defs = append(defs, d)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	return defs
}

// Get returns a server definition by ID.
func (s *MCPService) Get(id string) (*mcp.ServerDef, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d, ok := s.servers[id]
	if !ok {
		return nil, fmt.Errorf("mcp server %q: %w", id, domain.ErrNotFound)
	}
	return &d, nil
}

// Register validates and stores a server definition. If the ID is empty,
// a random ID is generated. Returns domain.ErrConflict if a server with
// the same ID already exists.
func (s *MCPService) Register(def mcp.ServerDef) error { //nolint:gocritic // hugeParam: value semantics for Register
	if err := def.Validate(); err != nil {
		return err
	}

	if def.ID == "" {
		def.ID = generateID()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[def.ID]; exists {
		return fmt.Errorf("mcp server %q: %w", def.ID, domain.ErrConflict)
	}

	if def.Status == "" {
		def.Status = mcp.ServerStatusRegistered
	}

	s.servers[def.ID] = def
	return nil
}

// Remove deletes a server definition by ID.
func (s *MCPService) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.servers[id]; !ok {
		return fmt.Errorf("mcp server %q: %w", id, domain.ErrNotFound)
	}
	delete(s.servers, id)
	return nil
}

// ResolveForRun returns all enabled server definitions. The projectID and
// modeID parameters are reserved for future DB-backed filtering (Phase 15C).
func (s *MCPService) ResolveForRun(_, _ string) []mcp.ServerDef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var defs []mcp.ServerDef
	for _, d := range s.servers { //nolint:gocritic // rangeValCopy: map iteration requires value copy
		if d.Enabled {
			defs = append(defs, d)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	return defs
}

// LoadFromDirectory reads all .yaml/.yml files from a directory and registers
// each as a server definition. A missing directory returns nil (not an error),
// matching the pattern in policy/loader.go.
func (s *MCPService) LoadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read mcp servers directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path built from trusted dir
		if readErr != nil {
			return fmt.Errorf("read mcp server file %s: %w", path, readErr)
		}

		var def mcp.ServerDef
		if unmarshalErr := yaml.Unmarshal(data, &def); unmarshalErr != nil {
			return fmt.Errorf("parse mcp server file %s: %w", path, unmarshalErr)
		}

		if regErr := s.Register(def); regErr != nil {
			return fmt.Errorf("register mcp server from %s: %w", path, regErr)
		}
	}

	return nil
}
