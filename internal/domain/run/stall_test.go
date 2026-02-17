package run

import "testing"

func TestStallTracker_ProgressResetsCounter(t *testing.T) {
	st := NewStallTracker(3)

	// Non-progress steps
	st.RecordStep("Read", true, "output1")
	st.RecordStep("Grep", true, "output2")

	// Progress step (Edit with success) resets counter
	stalled := st.RecordStep("Edit", true, "output3")
	if stalled {
		t.Fatal("expected no stall after progress step")
	}
	if st.IsStalled() {
		t.Fatal("expected IsStalled() = false after progress step")
	}
}

func TestStallTracker_ThresholdTriggers(t *testing.T) {
	st := NewStallTracker(3)

	st.RecordStep("Read", true, "a")
	st.RecordStep("Grep", true, "b")
	stalled := st.RecordStep("Read", true, "c")
	if !stalled {
		t.Fatal("expected stall after 3 consecutive non-progress steps")
	}
	if !st.IsStalled() {
		t.Fatal("expected IsStalled() = true")
	}
}

func TestStallTracker_FailedProgressToolNotCounted(t *testing.T) {
	st := NewStallTracker(3)

	// Edit with success=false is not progress
	st.RecordStep("Edit", false, "error")
	st.RecordStep("Edit", false, "error2")
	stalled := st.RecordStep("Read", true, "output")
	if !stalled {
		t.Fatal("expected stall: failed Edit doesn't count as progress")
	}
}

func TestStallTracker_RepetitionDetection(t *testing.T) {
	st := NewStallTracker(5)

	// Same output from Edit counts as no progress due to repetition
	st.RecordStep("Edit", true, "same output")
	stalled := st.RecordStep("Edit", true, "same output")
	if stalled {
		t.Fatal("should not stall yet (only 1 no-progress step)")
	}
	// The counter should be at 1 (second call was repeated, so no progress)
	if st.consecutiveNoProgress != 1 {
		t.Fatalf("expected 1 no-progress, got %d", st.consecutiveNoProgress)
	}
}

func TestStallTracker_DifferentOutputResets(t *testing.T) {
	st := NewStallTracker(3)

	st.RecordStep("Read", true, "output1") // no-progress (Read is not a progress tool)
	st.RecordStep("Read", true, "output2") // no-progress
	st.RecordStep("Bash", true, "result1") // progress — different output, success
	if st.consecutiveNoProgress != 0 {
		t.Fatalf("expected counter reset after progress, got %d", st.consecutiveNoProgress)
	}
}

func TestStallTracker_Reset(t *testing.T) {
	st := NewStallTracker(3)

	st.RecordStep("Read", true, "a")
	st.RecordStep("Read", true, "b")
	st.RecordStep("Read", true, "c")
	if !st.IsStalled() {
		t.Fatal("expected stall before reset")
	}

	st.Reset()
	if st.IsStalled() {
		t.Fatal("expected no stall after reset")
	}
	if st.consecutiveNoProgress != 0 {
		t.Fatalf("expected counter 0 after reset, got %d", st.consecutiveNoProgress)
	}
}

func TestStallTracker_DefaultThreshold(t *testing.T) {
	st := NewStallTracker(0) // should default to 5
	if st.threshold != 5 {
		t.Fatalf("expected default threshold 5, got %d", st.threshold)
	}

	st2 := NewStallTracker(-1) // should default to 5
	if st2.threshold != 5 {
		t.Fatalf("expected default threshold 5, got %d", st2.threshold)
	}
}

func TestStallTracker_WriteIsProgress(t *testing.T) {
	st := NewStallTracker(3)

	st.RecordStep("Read", true, "a")
	st.RecordStep("Read", true, "b")
	// Write with success resets
	stalled := st.RecordStep("Write", true, "wrote file")
	if stalled {
		t.Fatal("Write with success should be progress and prevent stall")
	}
	if st.consecutiveNoProgress != 0 {
		t.Fatalf("expected counter 0, got %d", st.consecutiveNoProgress)
	}
}

func TestStallTracker_BashIsProgress(t *testing.T) {
	st := NewStallTracker(3)

	st.RecordStep("Read", true, "a")
	st.RecordStep("Read", true, "b")
	// Bash with success resets
	stalled := st.RecordStep("Bash", true, "ran command")
	if stalled {
		t.Fatal("Bash with success should be progress and prevent stall")
	}
}

func TestStallTracker_RingBufferWraps(t *testing.T) {
	st := NewStallTracker(10)

	// Fill the ring buffer (size 3) and overflow
	st.RecordStep("Edit", true, "unique1")
	st.RecordStep("Edit", true, "unique2")
	st.RecordStep("Edit", true, "unique3")
	st.RecordStep("Edit", true, "unique4") // pushes out unique1

	// unique1 should no longer be in the buffer, so this should count as progress
	stalled := st.RecordStep("Edit", true, "unique1")
	if stalled {
		t.Fatal("ring buffer overflow: unique1 should no longer be detected as repeated")
	}
	if st.consecutiveNoProgress != 0 {
		t.Fatalf("expected counter 0, got %d", st.consecutiveNoProgress)
	}
}
