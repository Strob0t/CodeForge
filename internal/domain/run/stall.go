package run

import "hash/fnv"

// progressTools are tools that indicate meaningful work when successful.
var progressTools = map[string]bool{
	"Edit":  true,
	"Write": true,
	"Bash":  true,
}

// StallTracker monitors consecutive non-progress steps to detect agent stalls.
type StallTracker struct {
	consecutiveNoProgress int
	threshold             int
	recentHashes          [3]uint64
	recentCount           int
	retryCount            int
	maxRetries            int
}

// NewStallTracker creates a tracker that triggers after threshold consecutive no-progress steps.
// maxRetries controls how many re-plan attempts are allowed before giving up (P1-8).
func NewStallTracker(threshold, maxRetries int) *StallTracker {
	if threshold <= 0 {
		threshold = 5
	}
	if maxRetries < 0 {
		maxRetries = 2
	}
	return &StallTracker{threshold: threshold, maxRetries: maxRetries}
}

// CanRetry returns true if there are remaining retry attempts for re-planning.
func (s *StallTracker) CanRetry() bool {
	return s.retryCount < s.maxRetries
}

// RecordRetry increments the retry counter and resets stall state for a new attempt.
func (s *StallTracker) RecordRetry() {
	s.retryCount++
	s.Reset()
}

// RetryCount returns the number of re-plan attempts used so far.
func (s *StallTracker) RetryCount() int {
	return s.retryCount
}

// RecordStep records a tool execution and returns true if stalling is detected.
// Progress = Edit/Write/Bash with success=true and non-repeated output.
func (s *StallTracker) RecordStep(tool string, success bool, output string) bool {
	h := hashOutput(output)

	isProgress := progressTools[tool] && success
	isRepeated := s.isRepeatedOutput(h)

	if isProgress && !isRepeated {
		s.consecutiveNoProgress = 0
	} else {
		s.consecutiveNoProgress++
	}

	s.pushHash(h)
	return s.consecutiveNoProgress >= s.threshold
}

// IsStalled returns whether the tracker has detected a stall.
func (s *StallTracker) IsStalled() bool {
	return s.consecutiveNoProgress >= s.threshold
}

// Reset clears the stall counter and hash history.
func (s *StallTracker) Reset() {
	s.consecutiveNoProgress = 0
	s.recentHashes = [3]uint64{}
	s.recentCount = 0
}

func (s *StallTracker) pushHash(h uint64) {
	idx := s.recentCount % len(s.recentHashes)
	s.recentHashes[idx] = h
	s.recentCount++
}

func (s *StallTracker) isRepeatedOutput(h uint64) bool {
	if s.recentCount == 0 {
		return false
	}
	for i := range min(s.recentCount, len(s.recentHashes)) {
		if s.recentHashes[i] == h {
			return true
		}
	}
	return false
}

func hashOutput(output string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(output))
	return h.Sum64()
}
