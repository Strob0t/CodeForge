package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	lspAdapter "github.com/Strob0t/CodeForge/internal/adapter/lsp"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// LSPService manages language server clients per project.
type LSPService struct {
	cfg   *config.LSP
	hub   *ws.Hub
	store database.Store

	clients map[string]map[string]*lspAdapter.Client // projectID -> language -> client
	mu      sync.RWMutex

	// Debounce diagnostic broadcasts per projectID+URI.
	diagTimers map[string]*time.Timer
	diagMu     sync.Mutex
}

// NewLSPService creates a new LSP service.
func NewLSPService(cfg *config.LSP, hub *ws.Hub, store database.Store) *LSPService {
	return &LSPService{
		cfg:        cfg,
		hub:        hub,
		store:      store,
		clients:    make(map[string]map[string]*lspAdapter.Client),
		diagTimers: make(map[string]*time.Timer),
	}
}

// StartServers starts language servers for the detected languages in a project.
// If languages is nil, it auto-detects from the project workspace.
func (s *LSPService) StartServers(ctx context.Context, projectID, workspacePath string, languages []string) error {
	if workspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	// Auto-detect languages if none specified.
	if len(languages) == 0 {
		for lang := range lspDomain.DefaultServers {
			languages = append(languages, lang)
		}
	}

	s.mu.Lock()
	if s.clients[projectID] == nil {
		s.clients[projectID] = make(map[string]*lspAdapter.Client)
	}
	s.mu.Unlock()

	var started int
	for _, lang := range languages {
		cfg, ok := lspDomain.DefaultServers[lang]
		if !ok {
			slog.Debug("lsp: no server configured", "language", lang)
			continue
		}

		s.mu.RLock()
		existing := s.clients[projectID][lang]
		s.mu.RUnlock()
		if existing != nil && existing.Status() == lspDomain.ServerStatusReady {
			continue // Already running.
		}

		client := lspAdapter.NewClient(lang, cfg, s.cfg, workspacePath)
		client.SetDiagnosticCallback(func(uri string, diags []lspDomain.Diagnostic) {
			s.onDiagnostic(projectID, uri, diags)
		})

		// Broadcast starting status.
		s.broadcastStatus(ctx, projectID, lang, lspDomain.ServerStatusStarting, "")

		startCtx, cancel := context.WithTimeout(ctx, s.cfg.StartTimeout)
		if err := client.Start(startCtx); err != nil {
			cancel()
			slog.Warn("lsp: failed to start server", "language", lang, "error", err)
			s.broadcastStatus(ctx, projectID, lang, lspDomain.ServerStatusFailed, err.Error())
			continue
		}
		cancel()

		s.mu.Lock()
		s.clients[projectID][lang] = client
		s.mu.Unlock()

		s.broadcastStatus(ctx, projectID, lang, lspDomain.ServerStatusReady, "")
		started++
	}

	slog.Info("lsp servers started", "project_id", projectID, "count", started)
	return nil
}

// StopServers stops all language servers for a project.
func (s *LSPService) StopServers(ctx context.Context, projectID string) error {
	s.mu.Lock()
	clients := s.clients[projectID]
	delete(s.clients, projectID)
	s.mu.Unlock()

	if clients == nil {
		return nil
	}

	for lang, client := range clients {
		if err := client.Stop(ctx); err != nil {
			slog.Warn("lsp: failed to stop server", "language", lang, "error", err)
		}
		s.broadcastStatus(ctx, projectID, lang, lspDomain.ServerStatusStopped, "")
	}

	slog.Info("lsp servers stopped", "project_id", projectID)
	return nil
}

// Status returns the status of all language servers for a project.
func (s *LSPService) Status(projectID string) []lspDomain.ServerInfo {
	s.mu.RLock()
	clients := s.clients[projectID]
	s.mu.RUnlock()

	if clients == nil {
		return nil
	}

	infos := make([]lspDomain.ServerInfo, 0, len(clients))
	for _, client := range clients {
		infos = append(infos, lspDomain.ServerInfo{
			Language:    client.Language(),
			Status:      client.Status(),
			Command:     strings.Join(lspDomain.DefaultServers[client.Language()].Command, " "),
			PID:         client.PID(),
			Diagnostics: client.DiagnosticCount(),
		})
	}
	return infos
}

// Definition returns go-to-definition locations.
func (s *LSPService) Definition(ctx context.Context, projectID, uri string, pos lspDomain.Position) ([]lspDomain.Location, error) {
	client, err := s.clientForURI(projectID, uri)
	if err != nil {
		return nil, err
	}
	return client.Definition(ctx, uri, pos)
}

// References returns find-references locations.
func (s *LSPService) References(ctx context.Context, projectID, uri string, pos lspDomain.Position) ([]lspDomain.Location, error) {
	client, err := s.clientForURI(projectID, uri)
	if err != nil {
		return nil, err
	}
	return client.References(ctx, uri, pos)
}

// DocumentSymbols returns document symbols for a file.
func (s *LSPService) DocumentSymbols(ctx context.Context, projectID, uri string) ([]lspDomain.DocumentSymbol, error) {
	client, err := s.clientForURI(projectID, uri)
	if err != nil {
		return nil, err
	}
	return client.DocumentSymbols(ctx, uri)
}

// Hover returns hover information for a position.
func (s *LSPService) Hover(ctx context.Context, projectID, uri string, pos lspDomain.Position) (*lspDomain.HoverResult, error) {
	client, err := s.clientForURI(projectID, uri)
	if err != nil {
		return nil, err
	}
	return client.Hover(ctx, uri, pos)
}

// Diagnostics returns cached diagnostics for a project. If uri is non-empty, filters to that file.
func (s *LSPService) Diagnostics(projectID, uri string) []lspDomain.Diagnostic {
	s.mu.RLock()
	clients := s.clients[projectID]
	s.mu.RUnlock()

	if clients == nil {
		return nil
	}

	var all []lspDomain.Diagnostic
	for _, client := range clients {
		all = append(all, client.Diagnostics(uri)...)
	}
	return all
}

// DiagnosticsAsContextEntries returns all diagnostics for a project formatted as context entries.
// These are injected into context packs at priority 95 so agents see compilation errors.
func (s *LSPService) DiagnosticsAsContextEntries(projectID string) []cfcontext.ContextEntry {
	s.mu.RLock()
	clients := s.clients[projectID]
	s.mu.RUnlock()

	if clients == nil {
		return nil
	}

	var entries []cfcontext.ContextEntry
	for _, client := range clients {
		diagMap := client.AllDiagnostics()
		for uri, diags := range diagMap {
			if len(diags) == 0 {
				continue
			}

			var sb strings.Builder
			for _, d := range diags {
				severity := "INFO"
				switch d.Severity {
				case lspDomain.SeverityError:
					severity = "ERROR"
				case lspDomain.SeverityWarning:
					severity = "WARNING"
				case lspDomain.SeverityHint:
					severity = "HINT"
				}
				fmt.Fprintf(&sb, "%s:%d:%d: [%s] %s (%s)\n",
					uri, d.Range.Start.Line+1, d.Range.Start.Character+1,
					severity, d.Message, d.Source)
			}

			content := sb.String()
			entries = append(entries, cfcontext.ContextEntry{
				Kind:     cfcontext.EntryDiagnostic,
				Path:     uri,
				Content:  content,
				Tokens:   cfcontext.EstimateTokens(content),
				Priority: 95, // High priority — agents should see compilation errors.
			})
		}
	}

	return entries
}

// --- Internal ---

// clientForURI finds the appropriate language server client for a file URI.
// It infers language from the file extension.
func (s *LSPService) clientForURI(projectID, uri string) (*lspAdapter.Client, error) {
	s.mu.RLock()
	clients := s.clients[projectID]
	s.mu.RUnlock()

	if clients == nil {
		return nil, fmt.Errorf("no LSP servers running for project %s", projectID)
	}

	lang := languageFromURI(uri)
	if lang == "" {
		// Try first available client.
		for _, c := range clients {
			return c, nil
		}
		return nil, fmt.Errorf("no LSP server found for URI %s", uri)
	}

	client, ok := clients[lang]
	if !ok {
		return nil, fmt.Errorf("no LSP server running for language %s (URI: %s)", lang, uri)
	}
	return client, nil
}

// languageFromURI infers the programming language from a file URI's extension.
func languageFromURI(uri string) string {
	lower := strings.ToLower(uri)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"):
		return "javascript"
	default:
		return ""
	}
}

// onDiagnostic is the callback from individual clients when diagnostics are received.
// It debounces WS broadcasts.
func (s *LSPService) onDiagnostic(projectID, uri string, diags []lspDomain.Diagnostic) {
	key := projectID + "|" + uri

	s.diagMu.Lock()
	defer s.diagMu.Unlock()

	// Cancel any existing debounce timer.
	if t, ok := s.diagTimers[key]; ok {
		t.Stop()
	}

	// Set a new debounce timer.
	s.diagTimers[key] = time.AfterFunc(s.cfg.DiagnosticDelay, func() {
		s.hub.BroadcastEvent(context.Background(), ws.EventLSPDiagnostic, ws.LSPDiagnosticEvent{
			ProjectID:   projectID,
			URI:         uri,
			Diagnostics: diags,
		})

		s.diagMu.Lock()
		delete(s.diagTimers, key)
		s.diagMu.Unlock()
	})
}

// broadcastStatus sends an LSP status event via WebSocket.
func (s *LSPService) broadcastStatus(ctx context.Context, projectID, language string, status lspDomain.ServerStatus, errMsg string) {
	s.hub.BroadcastEvent(ctx, ws.EventLSPStatus, ws.LSPStatusEvent{
		ProjectID: projectID,
		Language:  language,
		Status:    string(status),
		Error:     errMsg,
	})
}
