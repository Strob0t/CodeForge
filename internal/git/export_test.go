package git

import (
	"testing"
	"time"
)

// CommandTimeout is the deadline of a git command (its first argument).
var CommandTimeout = commandTimeout

// SetCheckTimeout sets the deadline of OpenRepo's checks for one test.
func SetCheckTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := checkTimeout
	checkTimeout = d
	t.Cleanup(func() { checkTimeout = old })
}

// SetPreScanEntries sets the entry budget of OpenRepo's pre-scan of ignore
// and attributes files for one test.
func SetPreScanEntries(t *testing.T, n int) {
	t.Helper()
	old := maxPreScanEntries
	maxPreScanEntries = n
	t.Cleanup(func() { maxPreScanEntries = old })
}
