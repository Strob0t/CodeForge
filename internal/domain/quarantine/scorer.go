package quarantine

import (
	"regexp"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

// Precompiled patterns for content-based risk scoring.
var (
	shellPattern = regexp.MustCompile(`(;\s*rm\s|\|\s*curl\s|\|\s*wget\s|\|\s*bash|` + "`[^`]+`)")
	sqlPattern   = regexp.MustCompile(`(?i)(DROP\s+TABLE|DELETE\s+FROM|TRUNCATE|ALTER\s+TABLE|INSERT\s+INTO.*SELECT)`)
	pathPattern  = regexp.MustCompile(`\.\.[/\\]`)
	envPattern   = regexp.MustCompile(`(\$ENV|\$\{\w+\}|os\.environ|process\.env)`)
	b64Pattern   = regexp.MustCompile(`[A-Za-z0-9+/=]{100,}`)

	promptOverridePattern = regexp.MustCompile(
		`(?i)(ignore\s+(all\s+)?previous|disregard\s+(all\s+)?instructions|` +
			`you\s+are\s+now|forget\s+(everything|all)|new\s+instructions|` +
			`override\s+system|act\s+as\s+if|pretend\s+(you|that)|` +
			`do\s+not\s+follow|system\s+prompt\s+is)`)

	roleHijackPattern = regexp.MustCompile(
		`(?i)(from\s+now\s+on\s+you|switch\s+to\s+|change\s+your\s+behavior|` +
			`your\s+role\s+is\s+now)`)

	exfilPattern = regexp.MustCompile(
		`(?i)(send\s+to\s+https?://|exfiltrate|leak\s+(the|all)\s+)`)
)

// ScoreMessage computes a risk score for a message based on trust annotation
// and payload content. Returns a score in [0.0, 1.0] and human-readable factors.
func ScoreMessage(ann *trust.Annotation, payload []byte) (score float64, factors []string) {
	// Trust-based scoring.
	if ann != nil {
		switch ann.TrustLevel {
		case trust.LevelUntrusted:
			score += 0.5
			factors = append(factors, "untrusted source (+0.5)")
		case trust.LevelPartial:
			score += 0.2
			factors = append(factors, "partial trust (+0.2)")
		}
		if ann.Origin == "a2a" {
			score += 0.1
			factors = append(factors, "A2A origin (+0.1)")
		}
	}

	// Content-based scoring.
	body := string(payload)

	if shellPattern.MatchString(body) {
		score += 0.3
		factors = append(factors, "shell injection pattern detected")
	}
	if sqlPattern.MatchString(body) {
		score += 0.2
		factors = append(factors, "SQL injection pattern detected")
	}
	if pathPattern.MatchString(body) {
		score += 0.2
		factors = append(factors, "path traversal pattern detected")
	}
	if envPattern.MatchString(body) {
		score += 0.1
		factors = append(factors, "environment variable access detected")
	}
	if b64Pattern.MatchString(body) {
		score += 0.1
		factors = append(factors, "large base64 block detected")
	}
	if promptOverridePattern.MatchString(body) {
		score += 0.4
		factors = append(factors, "prompt override pattern detected")
	}
	if roleHijackPattern.MatchString(body) {
		score += 0.3
		factors = append(factors, "role hijack pattern detected")
	}
	if exfilPattern.MatchString(body) {
		score += 0.3
		factors = append(factors, "exfiltration pattern detected")
	}
	if strings.Count(body, "\"tool_call\"") > 10 || strings.Count(body, "tool_calls") > 10 {
		score += 0.1
		factors = append(factors, "excessive tool calls detected")
	}

	if score > 1.0 {
		score = 1.0
	}
	return score, factors
}
