package run

import "time"

// ToolCallRequest represents a worker requesting permission to execute a tool.
type ToolCallRequest struct {
	RunID   string `json:"run_id"`
	CallID  string `json:"call_id"`
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
}

// ToolCallResponse is the control plane's decision on a tool call request.
type ToolCallResponse struct {
	RunID    string `json:"run_id"`
	CallID   string `json:"call_id"`
	Decision string `json:"decision"` // "allow", "deny", "ask"
	Reason   string `json:"reason,omitempty"`
}

// ToolCallResult reports the outcome of an executed tool call.
type ToolCallResult struct {
	RunID    string        `json:"run_id"`
	CallID   string        `json:"call_id"`
	Success  bool          `json:"success"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	CostUSD  float64       `json:"cost_usd,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}
