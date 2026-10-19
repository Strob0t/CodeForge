package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// KI-96 D11: a project with active work is not deleted (its workspace is
// removed by the worker, after the work ended).
func TestWriteDomainError_ProjectBusyIs409(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDomainError(rec, fmt.Errorf("delete project p1: %w", project.ErrProjectBusy), "project not found")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "active work") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
