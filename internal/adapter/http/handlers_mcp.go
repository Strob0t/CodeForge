package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// --- MCP Server Handlers (Phase 15C) ---

// ListMCPServers handles GET /api/v1/mcp/servers
func (h *Handlers) ListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.MCP.ListDB(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if servers == nil {
		servers = []mcp.ServerDef{}
	}
	writeJSON(w, http.StatusOK, servers)
}

// GetMCPServer handles GET /api/v1/mcp/servers/{id}
func (h *Handlers) GetMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := h.MCP.GetDB(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "mcp server not found")
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

// CreateMCPServer handles POST /api/v1/mcp/servers
func (h *Handlers) CreateMCPServer(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[mcp.ServerDef](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	srv, err := h.MCP.CreateDB(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "create mcp server")
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

// UpdateMCPServer handles PUT /api/v1/mcp/servers/{id}
func (h *Handlers) UpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, ok := readJSON[mcp.ServerDef](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req.ID = id
	if err := h.MCP.UpdateDB(r.Context(), &req); err != nil {
		writeDomainError(w, err, "mcp server not found")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// DeleteMCPServer handles DELETE /api/v1/mcp/servers/{id}
func (h *Handlers) DeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.MCP.DeleteDB(r.Context(), id); err != nil {
		writeDomainError(w, err, "mcp server not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestMCPServer handles POST /api/v1/mcp/servers/{id}/test
// Performs a real MCP handshake against an existing server (re-reads config
// from DB, runs Initialize + ListTools, updates status).
func (h *Handlers) TestMCPServer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := h.MCP.GetDB(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "mcp server not found")
		return
	}
	result, err := h.MCP.TestConnection(r.Context(), srv)
	if err != nil {
		writeDomainError(w, err, "test mcp server")
		return
	}
	// Update the server status based on the test result.
	newStatus := mcp.ServerStatusConnected
	if !result.Success {
		newStatus = mcp.ServerStatusError
	}
	_ = h.MCP.UpdateStatusDB(r.Context(), id, newStatus)
	writeJSON(w, http.StatusOK, result)
}

// TestMCPServerConnection handles POST /api/v1/mcp/servers/test
// Accepts a ServerDef body (no ID needed) and performs a real MCP handshake
// to verify the server is reachable before saving. Returns discovered tools.
func (h *Handlers) TestMCPServerConnection(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[mcp.ServerDef](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	result, err := h.MCP.TestConnection(r.Context(), &req)
	if err != nil {
		writeDomainError(w, err, "test mcp server connection")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ListMCPServerTools handles GET /api/v1/mcp/servers/{id}/tools
func (h *Handlers) ListMCPServerTools(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tools, err := h.MCP.ListTools(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "mcp server not found")
		return
	}
	if tools == nil {
		tools = []mcp.ServerTool{}
	}
	writeJSON(w, http.StatusOK, tools)
}

// ListProjectMCPServers handles GET /api/v1/projects/{id}/mcp-servers
func (h *Handlers) ListProjectMCPServers(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	servers, err := h.MCP.ListByProject(r.Context(), projectID)
	if err != nil {
		writeDomainError(w, err, "list project mcp servers")
		return
	}
	if servers == nil {
		servers = []mcp.ServerDef{}
	}
	writeJSON(w, http.StatusOK, servers)
}

// assignMCPRequest is the request body for assigning an MCP server to a project.
type assignMCPRequest struct {
	ServerID string `json:"server_id"`
}

// AssignMCPServerToProject handles POST /api/v1/projects/{id}/mcp-servers
func (h *Handlers) AssignMCPServerToProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	req, ok := readJSON[assignMCPRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if req.ServerID == "" {
		writeError(w, http.StatusBadRequest, "server_id is required")
		return
	}
	if err := h.MCP.AssignToProject(r.Context(), projectID, req.ServerID); err != nil {
		writeDomainError(w, err, "assign mcp server to project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnassignMCPServerFromProject handles DELETE /api/v1/projects/{id}/mcp-servers/{serverId}
func (h *Handlers) UnassignMCPServerFromProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	serverID := chi.URLParam(r, "serverId")
	if err := h.MCP.UnassignFromProject(r.Context(), projectID, serverID); err != nil {
		writeDomainError(w, err, "unassign mcp server from project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
