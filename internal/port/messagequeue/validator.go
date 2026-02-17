package messagequeue

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Validate checks whether data is valid JSON conforming to the schema
// associated with the given subject. Unknown subjects pass validation
// (future-proof for new message types).
func Validate(subject string, data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON on subject %s", subject)
	}

	// Map subject to payload struct for structural validation.
	var target any
	switch {
	case subject == SubjectTaskCreated:
		target = &TaskCreatedPayload{}
	case subject == SubjectTaskResult:
		target = &TaskResultPayload{}
	case subject == SubjectTaskOutput:
		target = &TaskOutputPayload{}
	case subject == SubjectTaskCancel:
		target = &TaskCancelPayload{}
	case subject == SubjectAgentStatus:
		target = &AgentStatusPayload{}
	case strings.HasPrefix(subject, SubjectTaskAgent+"."):
		// tasks.agent.{backend} — the payload is a Task, not a custom schema.
		// Accept any valid JSON.
		return nil
	default:
		return nil
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("schema validation failed for %s: %w", subject, err)
	}
	return nil
}
