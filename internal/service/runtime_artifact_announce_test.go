package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// artifactRaceStore records stored artifact results and can let another path
// end a run just before it is completed or enters its quality gate.
type artifactRaceStore struct {
	*gateOutcomeStore

	mu                 sync.Mutex
	gateEndedElsewhere bool
	artifacts          []string // run IDs whose artifact result was stored
}

func (s *artifactRaceStore) EnterQualityGate(ctx context.Context, req *run.CompletionRequest) error {
	s.mu.Lock()
	elsewhere := s.gateEndedElsewhere
	s.mu.Unlock()
	if elsewhere {
		setStoredStatus(s.runtimeMockStore, req.ID, run.StatusCancelled)
	}
	return s.runtimeMockStore.EnterQualityGate(ctx, req)
}

func (s *artifactRaceStore) UpdateRunArtifact(_ context.Context, id, _ string, _ *bool, _ []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, id)
	return nil
}

func (s *artifactRaceStore) storedArtifacts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.artifacts...)
}

// diffModes is a mode provider whose modes require a DIFF artifact.
type diffModes struct{}

func (diffModes) Get(id string) (*mode.Mode, error) {
	return &mode.Mode{ID: id, Name: id, RequiredArtifact: "DIFF"}, nil
}

// KI-78: artifact-validation results, events and audit entries were written
// before CompleteRun, so a run that another path (a stop, the watchdog) ended
// first showed a validation it never finished with. They come only from the
// path whose end (or gate entry) won.
func TestArtifactValidation_AnnouncedOnlyByThePathThatWins(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		output     string
		lost       bool
		wantStatus run.Status
		wantValid  bool
	}{
		{name: "valid, no gate", profile: "plan-readonly", output: "+added line", wantStatus: run.StatusCompleted, wantValid: true},
		{name: "invalid, no gate", profile: "plan-readonly", output: "nothing to see", wantStatus: run.StatusFailed},
		{name: "valid, gate", profile: "headless-safe-sandbox", output: "+added line", wantStatus: run.StatusQualityGate, wantValid: true},
		{name: "valid, no gate, lost", profile: "plan-readonly", output: "+added line", lost: true, wantStatus: run.StatusCancelled},
		{name: "invalid, no gate, lost", profile: "plan-readonly", output: "nothing to see", lost: true, wantStatus: run.StatusCancelled},
		{name: "valid, gate, lost", profile: "headless-safe-sandbox", output: "+added line", lost: true, wantStatus: run.StatusCancelled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, mockStore, queue, bc := newRuntimeTestEnv()
			store := &artifactRaceStore{gateOutcomeStore: &gateOutcomeStore{runtimeMockStore: mockStore}}
			events := &recordingEventStore{}
			svc := service.NewRuntimeService(store, queue, bc, events, service.NewPolicyService("headless-safe-sandbox", nil), defaultGateConfig())
			svc.SetModeService(diffModes{})
			setStoredRun(mockStore, &run.Run{
				ID: "run-art", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: tc.profile, ModeID: "coder", Status: run.StatusRunning,
			})
			if tc.lost {
				store.endedElsewhere = run.StatusCancelled
				store.gateEndedElsewhere = true
			}

			if err := svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{
				RunID: "run-art", TaskID: "task-1", ProjectID: "proj-1", Status: "completed", Output: tc.output,
			}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}
			if r := storedRun(t, mockStore, "run-art"); r.Status != tc.wantStatus {
				t.Fatalf("run status = %s, want %s", r.Status, tc.wantStatus)
			}

			var validations []event.ArtifactValidationEvent
			for _, e := range bc.snapshot() {
				if v, ok := e.Data.(event.ArtifactValidationEvent); ok && e.EventType == event.EventArtifactValidation {
					validations = append(validations, v)
				}
			}
			var artifactEvents []event.Type
			for i := range events.events {
				if typ := events.events[i].Type; typ == event.TypeArtifactValidated || typ == event.TypeArtifactFailed {
					artifactEvents = append(artifactEvents, typ)
				}
			}
			var artifactAudits []string
			for i := range events.audits {
				if events.audits[i].Action == "artifact.failed" {
					artifactAudits = append(artifactAudits, events.audits[i].Action)
				}
			}

			if tc.lost {
				if stored := store.storedArtifacts(); len(stored) != 0 || len(validations) != 0 || len(artifactEvents) != 0 || len(artifactAudits) != 0 {
					t.Fatalf("losing path announced: stored %v, broadcasts %+v, events %v, audits %v", stored, validations, artifactEvents, artifactAudits)
				}
				return
			}

			if stored := store.storedArtifacts(); len(stored) != 1 {
				t.Errorf("stored artifact results = %v, want one", stored)
			}
			if len(validations) != 1 || validations[0].Valid != tc.wantValid || validations[0].ArtifactType != "DIFF" {
				t.Errorf("artifact.validation broadcasts = %+v, want one with valid=%t", validations, tc.wantValid)
			}
			wantEvent := event.TypeArtifactFailed
			if tc.wantValid {
				wantEvent = event.TypeArtifactValidated
			}
			if len(artifactEvents) != 1 || artifactEvents[0] != wantEvent {
				t.Errorf("artifact run events = %v, want [%s]", artifactEvents, wantEvent)
			}
			if wantAudits := map[bool]int{true: 0, false: 1}[tc.wantValid]; len(artifactAudits) != wantAudits {
				t.Errorf("artifact.failed audits = %d, want %d", len(artifactAudits), wantAudits)
			}
		})
	}
}
