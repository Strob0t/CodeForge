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
)
