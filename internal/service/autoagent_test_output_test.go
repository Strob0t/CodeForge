package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// S3-F review C7: the fix prompt hands the agent the tail of the test
// output (the summary and failures pytest prints last), bounded.
func TestTestOutputForPrompt(t *testing.T) {
	if got := testOutputForPrompt("=== 3 passed ==="); got != "=== 3 passed ===" {
		t.Fatalf("short output = %q, want it whole", got)
	}

	tail := "FAILED test_x.py::test_a\n=== 1 failed ===\n"
	got := testOutputForPrompt(strings.Repeat("early é line\n", 20_000) + tail)
	if len(got) > maxPromptTestOutput+100 {
		t.Fatalf("prompt output is %d bytes, want at most about %d", len(got), maxPromptTestOutput)
	}
	if !strings.HasSuffix(got, tail) || !strings.Contains(strings.SplitN(got, "\n", 2)[0], "truncated") {
		t.Fatalf("prompt output = %q..., want a truncation marker and the tail", got[:80])
	}
	if !utf8.ValidString(got) {
		t.Fatal("the cut split a UTF-8 character")
	}
}
