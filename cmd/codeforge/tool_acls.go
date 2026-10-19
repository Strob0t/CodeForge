package main

import (
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/config"
)

// toolACLsRequired reads workspace.tool_acls (KI-96, ADR-018) and logs what
// it selects: an unknown value fails closed (required), and production
// without per-tenant tool identities gets a warning.
func toolACLsRequired(cfg *config.Config) bool {
	required, known := cfg.Workspace.ToolACLsRequired()
	if !known {
		slog.Error("unknown workspace.tool_acls value, using required", "value", cfg.Workspace.ToolACLs)
	}
	switch {
	case required:
		slog.Info("per-tenant tool identities: tenant directories get POSIX ACLs and tool work carries the tenant's tool UID")
	case cfg.AppEnv == "production":
		slog.Warn("workspace.tool_acls is off in production: a worker that isolates tenants refuses the Core's tool work " +
			"(no tool_uid) and one that does not runs every tenant's tools as one user (KI-96); " +
			"set CODEFORGE_WORKSPACE_TOOL_ACLS=required")
	}
	return required
}
