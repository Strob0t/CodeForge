// Package database defines the database store port (interface).
package database

// Store is the composite port interface for database operations.
// It composes all role-specific store interfaces via embedding.
//
// ISP NOTE: This composite interface should only be used at the composition
// root (cmd/codeforge/main.go). Individual services should accept only the
// sub-interfaces they actually need (e.g., ProjectStore, RunStore) to follow
// the Interface Segregation Principle. The sub-interfaces below are designed
// for this purpose — each covers a single domain area with 2-15 methods.
// See ADR-014 for the planned migration path.
type Store interface {
	ProjectStore
	AgentStore
	TaskStore
	RunStore
	PlanStore
	ContextStore
	CostStore
	DashboardStore
	RoadmapStore
	TenantStore
	BranchProtectionStore
	UserStore
	AuthTokenStore
	ReviewStore
	SettingsStore
	VCSAccountStore
	LLMKeyStore
	ConversationStore
	MCPStore
	PromptStore
	BenchmarkStore
	MemoryStore
	MicroagentStore
	SkillStore
	FeedbackStore
	AutoAgentStore
	QuarantineStore
	RoutingStore
	A2AStore
	GoalStore
	ChannelStore
	BoundaryStore
	AuditStore
}
