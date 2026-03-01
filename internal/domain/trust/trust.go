// Package trust provides message-level trust annotations for inter-agent
// communication. Each message crossing a trust boundary carries an Annotation
// indicating its origin, trust level, and source identity.
//
// Trust levels (highest to lowest): full > verified > partial > untrusted.
//
// Internal messages are automatically stamped LevelFull. External messages
// (A2A, webhooks) default to LevelUntrusted until verified.
package trust

import "time"

// Level represents the trust level of a message source.
type Level string

const (
	// LevelFull indicates an internal CodeForge agent (auto-stamped).
	LevelFull Level = "full"
	// LevelVerified indicates an external agent with a valid cryptographic signature.
	LevelVerified Level = "verified"
	// LevelPartial indicates a known external source without signature verification.
	LevelPartial Level = "partial"
	// LevelUntrusted indicates an unknown or unverified origin.
	LevelUntrusted Level = "untrusted"
)

// levelRank maps each Level to a numeric rank for comparison.
var levelRank = map[Level]int{
	LevelFull:      3,
	LevelVerified:  2,
	LevelPartial:   1,
	LevelUntrusted: 0,
}

// Annotation carries trust metadata attached to an inter-agent message.
type Annotation struct {
	Origin     string `json:"origin"`              // "internal", "a2a", "mcp", "webhook"
	TrustLevel Level  `json:"trust_level"`         // Trust level of the source
	SourceID   string `json:"source_id"`           // Agent ID or external identifier
	Signature  string `json:"signature,omitempty"` // Ed25519 signature hex (future use)
	Timestamp  string `json:"timestamp"`           // RFC3339 timestamp
}

// Internal creates a trust annotation for an internal CodeForge agent.
func Internal(sourceID string) *Annotation {
	return &Annotation{
		Origin:     "internal",
		TrustLevel: LevelFull,
		SourceID:   sourceID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
}

// IsInternal returns true if the annotation represents a fully trusted internal source.
func (a *Annotation) IsInternal() bool {
	return a.Origin == "internal" && a.TrustLevel == LevelFull
}

// Rank returns the numeric rank of a trust level (higher = more trusted).
// Unknown levels return -1.
func Rank(l Level) int {
	r, ok := levelRank[l]
	if !ok {
		return -1
	}
	return r
}

// MeetsMinimum returns true if the annotation's trust level is at least
// the specified minimum. Returns false for unknown levels.
func (a *Annotation) MeetsMinimum(minLevel Level) bool {
	return Rank(a.TrustLevel) >= Rank(minLevel)
}
