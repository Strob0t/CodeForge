// Package messagequeue defines the message queue port (interface).
package messagequeue

import "context"

// Handler processes a message received from the queue.
// The context carries request-scoped values such as the request ID.
type Handler func(ctx context.Context, subject string, data []byte) error

// Queue is the port interface for publishing and subscribing to messages.
type Queue interface {
	// Publish sends a message to the given subject.
	Publish(ctx context.Context, subject string, data []byte) error

	// PublishWithDedup sends a message with a deduplication ID.
	// JetStream rejects messages with a Nats-Msg-Id it has seen within the
	// stream's Duplicates window, preventing duplicate processing.
	PublishWithDedup(ctx context.Context, subject string, data []byte, msgID string) error

	// Subscribe registers a handler for messages on the given subject.
	// The returned function cancels the subscription.
	Subscribe(ctx context.Context, subject string, handler Handler) (cancel func(), err error)

	// Drain gracefully drains all subscriptions before closing.
	// Pending messages are processed; no new messages are accepted.
	Drain() error

	// Close shuts down the queue connection immediately.
	Close() error

	// IsConnected reports whether the queue is currently connected.
	IsConnected() bool
}

// Subject constants for NATS subjects used by CodeForge.
const (
	SubjectTaskCreated = "tasks.created"
	SubjectTaskAgent   = "tasks.agent"  // tasks.agent.{backend} — dispatched to specific backend
	SubjectTaskResult  = "tasks.result" // results from workers
	SubjectTaskOutput  = "tasks.output" // streaming output lines from workers
	SubjectTaskCancel  = "tasks.cancel" // cancel a running task
	SubjectAgentStatus = "agents.status"
	SubjectAgentOutput = "agents.output" // Python → Go: per-line backend output

	// Run protocol subjects (Phase 4B step-by-step execution)
	SubjectRunStart            = "runs.start"             // Go → Python: start a new run
	SubjectRunToolCallRequest  = "runs.toolcall.request"  // Python → Go: request permission for tool call
	SubjectRunToolCallResponse = "runs.toolcall.response" // Go → Python: permission decision
	SubjectRunToolCallResult   = "runs.toolcall.result"   // Python → Go: tool execution result
	SubjectRunComplete         = "runs.complete"          // Python → Go: run finished
	SubjectRunCancel           = "runs.cancel"            // Go → Python: cancel a run
	SubjectRunOutput           = "runs.output"            // Python → Go: streaming output

	// Heartbeat subject (Phase 3C)
	SubjectRunHeartbeat = "runs.heartbeat" // Python → Go: periodic heartbeat

	// Quality gate subjects (Phase 4C)
	SubjectQualityGateRequest = "runs.qualitygate.request" // Go → Python: run tests/lint
	SubjectQualityGateResult  = "runs.qualitygate.result"  // Python → Go: gate outcome

	// Context subjects (Phase 5D)
	SubjectContextPacked = "context.packed"         // Go → Python: context pack ready for run
	SubjectSharedUpdated = "context.shared.updated" // Go → all: shared context changed

	// Context re-ranking subjects (Phase 3 — Context Intelligence)
	SubjectContextRerankRequest = "context.rerank.request" // Go → Python: rerank context entries
	SubjectContextRerankResult  = "context.rerank.result"  // Python → Go: reranked entries

	// RepoMap subjects (Phase 6A)
	SubjectRepoMapRequest = "repomap.generate.request" // Go → Python: request repo map generation
	SubjectRepoMapResult  = "repomap.generate.result"  // Python → Go: repo map generation result

	// Retrieval subjects (Phase 6B)
	SubjectRetrievalIndexRequest  = "retrieval.index.request"  // Go → Python: request index build
	SubjectRetrievalIndexResult   = "retrieval.index.result"   // Python → Go: index build result
	SubjectRetrievalSearchRequest = "retrieval.search.request" // Go → Python: search request
	SubjectRetrievalSearchResult  = "retrieval.search.result"  // Python → Go: search results

	// Retrieval Sub-Agent subjects (Phase 6C)
	SubjectSubAgentSearchRequest = "retrieval.subagent.request" // Go → Python: multi-query search
	SubjectSubAgentSearchResult  = "retrieval.subagent.result"  // Python → Go: multi-query results

	// GraphRAG subjects (Phase 6D)
	SubjectGraphBuildRequest  = "graph.build.request"  // Go → Python: request graph build
	SubjectGraphBuildResult   = "graph.build.result"   // Python → Go: graph build result
	SubjectGraphSearchRequest = "graph.search.request" // Go → Python: graph search request
	SubjectGraphSearchResult  = "graph.search.result"  // Python → Go: graph search results

	// MCP subjects (Phase 15A)
	SubjectMCPServerStatus  = "mcp.server.status"    // Python → Go: connection status update
	SubjectMCPToolDiscovery = "mcp.tools.discovered" // Python → Go: tools found on server

	// Conversation run subjects (Phase 17C)
	SubjectConversationRunStart       = "conversation.run.start"       // Go → Python: start a conversation run
	SubjectConversationRunComplete    = "conversation.run.complete"    // Python → Go: conversation run finished
	SubjectConversationRunCancel      = "conversation.run.cancel"      // Go → Python: cancel a conversation run
	SubjectConversationCompactRequest = "conversation.compact.request" // Go → Python: compact conversation history

	// Evaluation subjects (Phase 20G — GEMMAS)
	SubjectEvalGemmasRequest = "evaluation.gemmas.request" // Go → Python: compute GEMMAS metrics
	SubjectEvalGemmasResult  = "evaluation.gemmas.result"  // Python → Go: GEMMAS metric results

	// Agent identity subjects (Phase 23C)
	SubjectAgentMessage = "agents.message" // Go → Go: agent-to-agent inbox message

	// A2A subjects (Phase 27)
	SubjectA2ATaskCreated  = "a2a.task.created"  // Go → Python: inbound A2A task received
	SubjectA2ATaskComplete = "a2a.task.complete" // Python → Go: A2A task completed
	SubjectA2ATaskCancel   = "a2a.task.cancel"   // Go → Python: cancel A2A task

	// Benchmark subjects (Phase 26/28)
	SubjectBenchmarkRunRequest   = "benchmark.run.request"   // Go → Python: start benchmark execution
	SubjectBenchmarkRunResult    = "benchmark.run.result"    // Python → Go: benchmark execution results
	SubjectBenchmarkTaskStarted  = "benchmark.task.started"  // Python → Go: per-task start event
	SubjectBenchmarkTaskProgress = "benchmark.task.progress" // Python → Go: per-task completion + running totals

	// Memory subjects (Phase 23)
	SubjectMemoryStore        = "memory.store"         // Go → Python: store agent memory
	SubjectMemoryRecall       = "memory.recall"        // Go → Python: recall agent memories
	SubjectMemoryRecallResult = "memory.recall.result" // Python → Go: recall results

	// Handoff subjects (Phase 23B)
	SubjectHandoffRequest = "handoff.request" // Go → Python: agent-to-agent handoff

	// Trajectory subjects (Phase 6.1)
	SubjectTrajectoryEvent = "runs.trajectory.event" // Python → Go: granular trajectory events

	// Backend health subjects (Phase 5.4)
	SubjectBackendHealthRequest = "backends.health.request" // Go → Python: check backend availability
	SubjectBackendHealthResult  = "backends.health.result"  // Python → Go: health check results

	// Review/Refactor subjects (Phase 31)
	SubjectReviewTriggerRequest   = "review.trigger.request"   // Go → Python: trigger a review run
	SubjectReviewTriggerComplete  = "review.trigger.complete"  // Python → Go: review run finished
	SubjectReviewBoundaryAnalyzed = "review.boundary.analyzed" // Python → Go: layer boundaries detected
	SubjectReviewApprovalRequired = "review.approval.required" // Python → Go: human approval needed
	SubjectReviewApprovalResponse = "review.approval.response" // Go → Python: approval decision

	// Prompt evolution subjects (Phase 33)
	SubjectPromptEvolutionReflect         = "prompt.evolution.reflect"          // Go → Python: request failure reflection
	SubjectPromptEvolutionReflectComplete = "prompt.evolution.reflect.complete" // Python → Go: reflection results
	SubjectPromptEvolutionMutateComplete  = "prompt.evolution.mutate.complete"  // Python → Go: mutation results
	SubjectPromptEvolutionPromoted        = "prompt.evolution.promoted"         // Go event: variant promoted
	SubjectPromptEvolutionReverted        = "prompt.evolution.reverted"         // Go event: variant reverted
)
