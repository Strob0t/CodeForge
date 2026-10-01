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
