package service

import "time"

// Test-only access to RuntimeService internals for the external
// service_test package; not part of the production API.

// SetHeartbeat sets the last heartbeat kept in memory for a run.
func (s *RuntimeService) SetHeartbeat(runID string, t time.Time) {
	s.state.SetHeartbeat(runID, t)
}

// LastHeartbeat returns the last heartbeat kept in memory for a run.
func (s *RuntimeService) LastHeartbeat(runID string) (time.Time, bool) {
	return s.state.GetHeartbeat(runID)
}

// WaitForReviews waits until the step reviews being decided are done.
func (s *OrchestratorService) WaitForReviews() {
	s.reviews.Wait()
}

// ReviewDecisionCount returns how many decided reviews wait for their step's start.
func (s *OrchestratorService) ReviewDecisionCount() int {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	return len(s.reviewDecisions)
}

// PreparedOutcomeCount returns how many step preparations wait for their step's start.
func (s *OrchestratorService) PreparedOutcomeCount() int {
	s.prepMu.Lock()
	defer s.prepMu.Unlock()
	return len(s.prepared)
}
