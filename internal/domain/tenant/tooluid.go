package tenant

import "errors"

// Tool identities (KI-96, ADR-018). Every tenant that runs agent tools gets
// its own tool UID: the worker starts the tenant's tool processes as that UID
// and GID, and the tenant directories grant it access through POSIX ACLs.
// The worker (codeforge/tool_identity.py) holds the same values; a contract
// test pins them.
const (
	// ToolUIDMin and ToolUIDMax bound the tenant tool UIDs (tenants.tool_uid).
	ToolUIDMin = 20000
	ToolUIDMax = 29999
	// SystemToolUID runs the worker's tenantless spawns (the isolation
	// probe, CLI checks); it has no access to any workspace.
	SystemToolUID = 19999
	// LegacyToolUID is the retired shared tool user of KI-71. It still owns
	// files from before the upgrade; no tool process runs as it.
	LegacyToolUID = 10002
)

// ErrToolUIDRangeExhausted is returned when every tool UID is allocated.
var ErrToolUIDRangeExhausted = errors.New("tool UID range exhausted (10,000 tenants with tool work)")

// IsToolUID reports whether uid is a tenant tool UID.
func IsToolUID(uid int) bool {
	return uid >= ToolUIDMin && uid <= ToolUIDMax
}

// ErrDisabled is returned for a tenant whose enabled flag is off: every
// authenticated request of its users is refused (KI-174).
var ErrDisabled = errors.New("tenant is disabled")
