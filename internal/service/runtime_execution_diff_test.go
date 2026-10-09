package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// The cap is a budget for the content a diff's hunks carry: content of
// exactly the budget passes whole, one byte more is cut and marked.
func TestCappedDiff_Boundary(t *testing.T) {
	diffWithContent := func(t *testing.T, content string) json.RawMessage {
		t.Helper()
		out, err := json.Marshal(broadcastDiff{Path: "a.go", Hunks: []broadcastDiffHunk{
			{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, OldContent: "", NewContent: content},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	tests := []struct {
		name        string
		contentSize int
		wantContent int
	}{
		{"content at the budget", maxBroadcastDiffContentBytes, maxBroadcastDiffContentBytes},
		{"content one byte over the budget", maxBroadcastDiffContentBytes + 1, maxBroadcastDiffContentBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := strings.Repeat("x", tt.contentSize)
			got := cappedDiff(diffWithContent(t, content))
			var d broadcastDiff
			if err := json.Unmarshal(got, &d); err != nil {
				t.Fatalf("broadcast diff is no diff: %v", err)
			}
			if d.Path != "a.go" || len(d.Hunks) != 1 {
				t.Fatalf("diff = path %q, %d hunks; want a.go with its hunk", d.Path, len(d.Hunks))
			}
			wantTruncated := tt.wantContent < tt.contentSize
			if d.Truncated != wantTruncated {
				t.Errorf("truncated = %t, want %t", d.Truncated, wantTruncated)
			}
			if d.Hunks[0].NewContent != content[:tt.wantContent] {
				t.Errorf("hunk content is %d bytes, want the first %d bytes", len(d.Hunks[0].NewContent), tt.wantContent)
			}
		})
	}
}

// A diff within the cap is broadcast as the worker sent it; large content
// that is no diff does not reach the browser.
func TestCappedDiff_NoDiff(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"nil", nil, ""},
		{"empty", json.RawMessage(""), ""},
		{"small object", json.RawMessage(`{"path":"a.go"}`), `{"path":"a.go"}`},
		{"large non-diff", json.RawMessage(`"` + strings.Repeat("x", maxBroadcastDiffContentBytes) + `"`), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(cappedDiff(tt.raw)); got != tt.want {
				t.Errorf("cappedDiff = %q, want %q", got, tt.want)
			}
		})
	}
}
