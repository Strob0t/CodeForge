package event

// Broadcast event type constants for WebSocket messages.
const (
	EventTaskStatus  = "task.status"
	EventTaskOutput  = "task.output"
	EventAgentStatus = "agent.status"

	// Run protocol events (Phase 4B)
	EventRunStatus      = "run.status"
	EventToolCallStatus = "run.toolcall"

	// Phase 4C events
	EventQualityGate = "run.qualitygate"
	EventDelivery    = "run.delivery"

	// Phase 12E: artifact validation events
	EventArtifactValidation = "run.artifact"

	// Phase 5A: orchestration plan events
	EventPlanStatus     = "plan.status"
	EventPlanStepStatus = "plan.step.status"

	// Phase 5E: team + shared context events
	EventTeamStatus          = "team.status"
	EventSharedContextUpdate = "shared.updated"

	// Phase 6A: repo map events
	EventRepoMapStatus = "repomap.status"

	// Phase 6B: retrieval events
	EventRetrievalStatus = "retrieval.status"

	// Phase 6D: graph events
	EventGraphStatus = "graph.status"

	// Phase 7: cost transparency events
	EventBudgetAlert = "run.budget_alert"

	// Phase 8: roadmap events
	EventRoadmapStatus = "roadmap.status"

	// Phase 8B: VCS webhook events
	EventVCSPush        = "vcs.push"
	EventVCSPullRequest = "vcs.pull_request"

	// Phase 12I: review events
	EventReviewStatus           = "review.status"
	EventReviewApprovalRequired = "review.approval_required"

	// Phase 13.5A: conversation events
	EventConversationMessage = "conversation.message"

	// LSP: language server events
	EventLSPStatus     = "lsp.status"
	EventLSPDiagnostic = "lsp.diagnostic"

	// Phase 21A: review router events
	EventReviewRouterDecision = "review_router.decision"

	// Phase 21D: debate events
	EventDebateStatus = "debate.status"

	// Phase 22: model health events
	EventModelHealth = "models.health"

	// Phase 23B: quarantine events
	EventQuarantineAlert    = "quarantine.alert"
	EventQuarantineResolved = "quarantine.resolved"

	// Phase 23C: agent identity events
	EventAgentMessage = "agent.message"

	// Phase 24: active work visibility events
	EventActiveWorkClaimed  = "activework.claimed"
	EventActiveWorkReleased = "activework.released"

	// Phase 23D: handoff status events (War Room)
	EventHandoffStatus = "handoff.status"

	// Phase 26G: benchmark progress events
	EventBenchmarkTaskStarted   = "benchmark.task.started"
	EventBenchmarkTaskCompleted = "benchmark.task.completed"
	EventBenchmarkRunProgress   = "benchmark.run.progress"

	// Phase 27: A2A protocol events
	EventA2ATaskCreated  = "a2a.task.created"
	EventA2ATaskStatus   = "a2a.task.status"
	EventA2ATaskComplete = "a2a.task.complete"

	// Trajectory events
	EventTrajectoryEvent = "trajectory.event"

	// Auto-Agent Skills: skill draft notification
	EventSkillDraft = "skill.draft"

	// Phase 5.2: agent backend output streaming
	EventAgentOutput = "agent.output"

	// Channel events
	EventChannelMessage = "channel.message"
	EventChannelTyping  = "channel.typing"
	EventChannelRead    = "channel.read"
)
