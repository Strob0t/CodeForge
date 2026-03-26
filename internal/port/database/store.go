// Package database defines the database store port (interface).
package database

// Store is the port interface for database operations.
// It composes all role-specific store interfaces via embedding.
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
