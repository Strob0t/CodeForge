package markdownspec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// todoFixture is a TODO.md with everything the old re-render lost: an
// intro with a link, nested and described items, a code block with a
// heading and a checkbox in it, a numbered list, a #### heading, a table,
// an uppercase [X], CRLF lines and a line longer than bufio.Scanner's
// 64 KiB limit.
func todoFixture() string {
	return "# Project TODO\n\nSome intro paragraph with [a link](http://x).\n\n## Phase 1\n\n" +
		"- [x] Done item\n  details of done item\n- [ ] Open item\n    - [ ] Nested item\n\n" +
		"```go\n# not a heading\n- [ ] not an item\nfunc main() {}\n```\n\n" +
		"1. numbered step\n2. another step\n#### Deep heading\n| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		strings.Repeat("long ", 20000) + "\n" +
		"* [X] Upper done\r\n- [ ] After the long line\n<!-- trailing comment -->"
}

// importedItems are the checkbox items of content as ParseItems reports them.
func importedItems(t *testing.T, content string) []specprovider.SpecItemDetail {
	t.Helper()
	items, err := (&Provider{}).ParseItems([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	var boxes []specprovider.SpecItemDetail
	for _, it := range items {
		if it.Level == string(LevelCheckbox) {
			boxes = append(boxes, it)
		}
	}
	return boxes
}

// KI-203 (R4-2): "Sync to file" re-rendered the whole file. Writing the
// statuses back as imported changes nothing, byte for byte, and [x] stays [x].
func TestPatchItems_RoundTripIsByteIdentical(t *testing.T) {
	content := todoFixture()
	items := importedItems(t, content)
	titles := make([]string, 0, len(items))
	for _, it := range items {
		titles = append(titles, it.Title)
	}
	want := "Done item|Open item|Nested item|Upper done|After the long line"
	if strings.Join(titles, "|") != want {
		t.Fatalf("checkbox items = %v, want %s", titles, want)
	}

	got, err := (&Provider{}).PatchItems([]byte(content), items)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(content)) {
		t.Fatalf("round trip changed the file:\n%q", firstDifference(got, []byte(content)))
	}
}

// Only the markers of changed statuses change: one byte per changed item.
func TestPatchItems_ChangesOnlyTheMarkers(t *testing.T) {
	content := todoFixture()
	items := importedItems(t, content)
	for i := range items {
		switch items[i].Title {
		case "Done item":
			items[i].Status = string(StatusTodo)
		case "Open item", "After the long line":
			items[i].Status = string(StatusDone)
		case "Nested item":
			items[i].Status = string(StatusInProgress) // no marker for it: stays unchecked
		}
	}
	got, err := (&Provider{}).PatchItems([]byte(content), items)
	if err != nil {
		t.Fatal(err)
	}
	want := content
	want = strings.Replace(want, "- [x] Done item", "- [ ] Done item", 1)
	want = strings.Replace(want, "- [ ] Open item", "- [x] Open item", 1)
	want = strings.Replace(want, "- [ ] After the long line", "- [x] After the long line", 1)
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("patched file differs from the expected markers:\n%q", firstDifference(got, []byte(want)))
	}
	// An uppercase [X] that stays done is kept as written.
	if !strings.Contains(string(got), "* [X] Upper done\r\n") {
		t.Fatal("the uppercase marker or the CRLF line ending was changed")
	}
}

// A line that no longer holds a checkbox is never written: the whole patch
// fails and nothing is returned.
func TestPatchItems_RefusesMovedItems(t *testing.T) {
	content := todoFixture()
	open := func(line int, title string) []specprovider.SpecItemDetail {
		return []specprovider.SpecItemDetail{
			{Title: "Done item", Status: "todo", SourceLine: 7, Level: "checkbox"},
			{Title: title, Status: "done", SourceLine: line, Level: "checkbox"},
		}
	}
	tests := []struct {
		name  string
		items []specprovider.SpecItemDetail
	}{
		{"a heading line", open(5, "Phase 1")},
		{"a checkbox inside a code block", open(14, "not an item")},
		{"a numbered list line", open(18, "numbered step")},
		{"line zero", open(0, "Open item")},
		{"negative line", open(-1, "Open item")},
		{"past the end", open(1000, "Open item")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&Provider{}).PatchItems([]byte(content), tt.items)
			if !errors.Is(err, specprovider.ErrItemMoved) || got != nil {
				t.Fatalf("PatchItems = %d bytes, %v; want ErrItemMoved and no content", len(got), err)
			}
		})
	}
}

// A checkbox whose title differs from the item's (a feature renamed in the
// UI) is patched: the caller made sure the file did not change since it was
// parsed (KI-203), so the line still holds the item.
func TestPatchItems_RenamedItem(t *testing.T) {
	content := todoFixture()
	got, err := (&Provider{}).PatchItems([]byte(content), []specprovider.SpecItemDetail{
		{Title: "Renamed item", Status: "done", SourceLine: 9, Level: "checkbox"},
	})
	if err != nil {
		t.Fatalf("PatchItems = %v", err)
	}
	if want := strings.Replace(content, "- [ ] Open item", "- [x] Open item", 1); string(got) != want {
		t.Fatalf("PatchItems changed more than the marker: %s", firstDifference(got, []byte(want)))
	}
}

func TestPatchItems_NothingToPatch(t *testing.T) {
	for _, content := range []string{"", "# Only a heading\n"} {
		got, err := (&Provider{}).PatchItems([]byte(content), nil)
		if err != nil || !bytes.Equal(got, []byte(content)) {
			t.Fatalf("PatchItems(%q, nil) = %q, %v", content, got, err)
		}
	}
}

// firstDifference shows the bytes around the first difference of a and b.
func firstDifference(a, b []byte) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := max(0, i-40)
	return fmt.Sprintf("at byte %d: got %q, want %q", i, a[lo:min(len(a), i+40)], b[lo:min(len(b), i+40)])
}
