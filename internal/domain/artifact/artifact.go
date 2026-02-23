// Package artifact defines typed artifact schemas and structural validators
// for run output produced by agent modes.
package artifact

import (
	"encoding/json"
	"strings"
)

// ArtifactType identifies the kind of artifact a mode is expected to produce.
type ArtifactType string

const (
	TypePlanMD      ArtifactType = "PLAN.md"
	TypeDiff        ArtifactType = "DIFF"
	TypeReviewMD    ArtifactType = "REVIEW.md"
	TypeTestReport  ArtifactType = "TEST_REPORT"
	TypeAuditReport ArtifactType = "AUDIT_REPORT"
	TypeDecisionMD  ArtifactType = "DECISION.md"
)

// knownTypes maps artifact type strings to their validator function.
var knownTypes = map[ArtifactType]func(string) []string{
	TypePlanMD:      validatePlanMD,
	TypeDiff:        validateDiff,
	TypeReviewMD:    validateReviewMD,
	TypeTestReport:  validateTestReport,
	TypeAuditReport: validateAuditReport,
	TypeDecisionMD:  validateDecisionMD,
}

// ValidationResult holds the outcome of validating run output against an artifact schema.
type ValidationResult struct {
	Valid        bool         `json:"valid"`
	ArtifactType ArtifactType `json:"artifact_type"`
	Errors       []string     `json:"errors,omitempty"`
}

// IsKnownType reports whether t is a recognized artifact type.
func IsKnownType(t string) bool {
	_, ok := knownTypes[ArtifactType(t)]
	return ok
}

// Validate checks that output conforms to the structural requirements of the given artifact type.
// An empty artifactType returns Valid=true (no artifact required).
func Validate(artifactType, output string) ValidationResult {
	if artifactType == "" {
		return ValidationResult{Valid: true}
	}

	at := ArtifactType(artifactType)
	fn, ok := knownTypes[at]
	if !ok {
		return ValidationResult{
			ArtifactType: at,
			Errors:       []string{"unknown artifact type: " + artifactType},
		}
	}

	errs := fn(output)
	return ValidationResult{
		Valid:        len(errs) == 0,
		ArtifactType: at,
		Errors:       errs,
	}
}

// --- Per-type validators ---

func validatePlanMD(output string) []string {
	var errs []string
	if len(output) < 100 {
		errs = append(errs, "PLAN.md must be at least 100 characters")
	}
	if !strings.Contains(output, "##") {
		errs = append(errs, "PLAN.md must contain at least one markdown H2 header (##)")
	}
	return errs
}

func validateDiff(output string) []string {
	hasDiffMarkers := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			hasDiffMarkers = true
			break
		}
	}
	if hasDiffMarkers {
		return nil
	}
	if strings.Contains(strings.ToLower(output), "no changes") {
		return nil
	}
	return []string{"DIFF must contain diff markers (lines starting with +/-) or indicate 'no changes'"}
}

func validateReviewMD(output string) []string {
	var errs []string
	if len(output) < 50 {
		errs = append(errs, "REVIEW.md must be at least 50 characters")
	}
	if !strings.Contains(output, "#") {
		errs = append(errs, "REVIEW.md must contain at least one markdown header (#)")
	}
	lower := strings.ToLower(output)
	keywords := []string{"finding", "issue", "comment", "suggestion", "recommendation"}
	found := false
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			found = true
			break
		}
	}
	if !found {
		errs = append(errs, "REVIEW.md must contain at least one review keyword (finding, issue, comment, suggestion, recommendation)")
	}
	return errs
}

func validateTestReport(output string) []string {
	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		return []string{"TEST_REPORT must be valid JSON"}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return []string{"TEST_REPORT must be a JSON object"}
	}
	_, hasStatus := obj["status"]
	_, hasPassed := obj["passed"]
	if !hasStatus && !hasPassed {
		return []string{"TEST_REPORT JSON must contain a 'status' or 'passed' key"}
	}
	return nil
}

func validateAuditReport(output string) []string {
	var errs []string
	if len(output) < 50 {
		errs = append(errs, "AUDIT_REPORT must be at least 50 characters")
	}
	if !strings.Contains(output, "#") {
		errs = append(errs, "AUDIT_REPORT must contain at least one markdown header (#)")
	}
	lower := strings.ToLower(output)
	keywords := []string{"vulnerability", "risk", "finding", "security"}
	found := false
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			found = true
			break
		}
	}
	if !found {
		errs = append(errs, "AUDIT_REPORT must contain at least one security keyword (vulnerability, risk, finding, security)")
	}
	return errs
}

func validateDecisionMD(output string) []string {
	var errs []string
	if len(output) < 50 {
		errs = append(errs, "DECISION.md must be at least 50 characters")
	}
	lower := strings.ToLower(output)
	if !strings.Contains(lower, "decision") {
		errs = append(errs, "DECISION.md must contain the word 'decision'")
	}
	if !strings.Contains(lower, "rationale") && !strings.Contains(lower, "reason") {
		errs = append(errs, "DECISION.md must contain the word 'rationale' or 'reason'")
	}
	return errs
}
