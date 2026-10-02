package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
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

	// outbound decides which addresses sse and streamable_http servers may
	// use (KI-100); allowedPrivateHosts is its allowlist, as configured.
	outbound            *netutil.OutboundPolicy
	allowedPrivateHosts []string
}

// NewMCPService creates an MCPService. If cfg.ServersDir is set, definitions
// are loaded from that directory on creation.
func NewMCPService(cfg *config.MCP, limits *config.Limits) *MCPService {
	allowed := slices.Clone(cfg.AllowedPrivateHosts)
	outbound, err := netutil.NewOutboundPolicy(allowed)
	if err != nil {
		// config.Load refuses such a list; without it, no private address is allowed.
		slog.Error("invalid mcp.allowed_private_hosts: every private address is refused", "error", err)
		allowed = nil
		outbound, _ = netutil.NewOutboundPolicy(nil)
	}
	s := &MCPService{
		servers:             make(map[string]mcp.ServerDef),
		serversDir:          cfg.ServersDir,
		limits:              limits,
		outbound:            outbound,
		allowedPrivateHosts: allowed,
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
		def.ID = crypto.GenerateUUIDv4()
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

// ResolveForRun returns MCP server definitions available for a run.
// It merges globally-enabled YAML servers with DB-assigned project servers.
// If projectID is non-empty and the DB is configured, the project's servers
// of the run's tenant (the tenant in ctx) are included. The modeID
// parameter is reserved for future filtering.
func (s *MCPService) ResolveForRun(ctx context.Context, projectID, _ string) []mcp.ServerDef {
	resolved := s.resolveForRun(ctx, projectID)
	defs := make([]mcp.ServerDef, len(resolved))
	for i := range resolved {
		defs[i] = resolved[i].def
	}
	return defs
}

// runServer is a server of a run and whether the operator defined it
// (servers_dir) rather than a tenant (the database).
type runServer struct {
	def      mcp.ServerDef
	operator bool
}

// resolveForRun is ResolveForRun with the origin of each server. An operator
// server wins over a stored one with the same ID.
func (s *MCPService) resolveForRun(ctx context.Context, projectID string) []runServer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var servers []runServer

	// Include globally-enabled YAML-loaded servers.
	for _, d := range s.servers { //nolint:gocritic // rangeValCopy: map iteration requires value copy
		if d.Enabled {
			servers = append(servers, runServer{def: d, operator: true})
			seen[d.ID] = true
		}
	}

	// Include DB-assigned project servers (if DB is configured and projectID given).
	if projectID != "" && s.db != nil {
		dbDefs, err := s.db.ListMCPServersByProject(ctx, projectID)
		if err != nil {
			slog.WarnContext(ctx, "resolve mcp servers for project", "project_id", projectID, "error", err)
		} else {
			for i := range dbDefs {
				if dbDefs[i].Enabled && !seen[dbDefs[i].ID] {
					servers = append(servers, runServer{def: dbDefs[i]})
					seen[dbDefs[i].ID] = true
				}
			}
		}
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].def.ID < servers[j].def.ID
	})
	return servers
}

// RunServerPayloads returns the NATS payloads of the servers ResolveForRun
// returns, for runs and conversations alike. sse and streamable_http
// servers carry mcp.allowed_private_hosts: the worker applies the outbound
// rules of the Go Core to every connection it opens (KI-100). Operator
// servers (servers_dir) are marked trusted: the worker lets them use private
// and loopback addresses; a tenant's server never is.
func (s *MCPService) RunServerPayloads(ctx context.Context, projectID, _ string) []messagequeue.MCPServerDefPayload {
	servers := s.resolveForRun(ctx, projectID)
	payloads := make([]messagequeue.MCPServerDefPayload, 0, len(servers))
	for i := range servers {
		d := &servers[i].def
		p := messagequeue.MCPServerDefPayload{
			ID:          d.ID,
			Name:        d.Name,
			Description: d.Description,
			Transport:   string(d.Transport),
			Command:     d.Command,
			Args:        d.Args,
			URL:         d.URL,
			Env:         d.Env,
			Headers:     d.Headers,
			Enabled:     d.Enabled,
		}
		if d.Transport == mcp.TransportSSE || d.Transport == mcp.TransportStreamableHTTP {
			p.AllowedPrivateHosts = s.allowedPrivateHosts
			p.Trusted = servers[i].operator
		}
		payloads = append(payloads, p)
	}
	return payloads
}

// LoadFromDirectory reads all .yaml/.yml files from a directory and registers
// each as a server definition. A file that cannot be read, parsed or
// registered is logged at error level with its name and reason and skipped;
// the others still load. A missing directory returns nil (not an error),
// matching the pattern in policy/loader.go; an unreadable one is an error.
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
		if err := s.loadFile(path); err != nil {
			slog.Error("skipping mcp server definition", "file", path, "error", err)
		}
	}

	return nil
}

// loadFile registers the server definition of one YAML file.
func (s *MCPService) loadFile(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path built from the operator's servers_dir
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	var def mcp.ServerDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if err := s.Register(def); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	return nil
}
