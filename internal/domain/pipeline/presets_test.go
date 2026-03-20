package pipeline

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

func TestBuiltinTemplatesContainReviewRefactor(t *testing.T) {
	templates := BuiltinTemplates()
	var found bool
	for _, tmpl := range templates {
		if tmpl.ID != "review-refactor" {
			continue
		}
		found = true
		if tmpl.Protocol != plan.ProtocolSequential {
			t.Errorf("protocol = %q, want sequential", tmpl.Protocol)
		}
		if len(tmpl.Steps) != 4 {
			t.Errorf("steps = %d, want 4", len(tmpl.Steps))
		}
		if tmpl.Steps[0].ModeID != "boundary_analyzer" {
			t.Errorf("step[0].ModeID = %q, want boundary_analyzer", tmpl.Steps[0].ModeID)
		}
		if tmpl.Steps[1].ModeID != "contract_reviewer" {
			t.Errorf("step[1].ModeID = %q, want contract_reviewer", tmpl.Steps[1].ModeID)
		}
		if tmpl.Steps[2].ModeID != "reviewer" {
			t.Errorf("step[2].ModeID = %q, want reviewer", tmpl.Steps[2].ModeID)
		}
		if tmpl.Steps[3].ModeID != "refactorer" {
			t.Errorf("step[3].ModeID = %q, want refactorer", tmpl.Steps[3].ModeID)
		}
	}
	if !found {
		t.Error("review-refactor template not found in BuiltinTemplates()")
	}
}
