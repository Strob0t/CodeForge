package markdownspec

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// patchCheckboxes returns content with the checkbox marker of each item's
// line set to the item's status: "[x]" for done, "[ ]" for any other
// status. A marker that already says so is kept as written ("[X]" stays).
// Only marker bytes change (KI-203: the file was rendered anew and lost
// everything that was not a heading or a list item). Each item's line must
// still hold a checkbox, outside code blocks; otherwise nothing is returned
// and the error wraps ErrItemMoved. Its title may differ from the item's
// (a feature renamed in the UI): the caller checks that the file did not
// change since it was parsed.
func patchCheckboxes(content []byte, items []specprovider.SpecItemDetail) ([]byte, error) {
	boxes := make(map[int]bool)
	for _, it := range ParseMarkdown(content) {
		if it.Level == LevelCheckbox {
			boxes[it.SourceLine] = true
		}
	}
	var starts []int // byte offset of each line
	offset := 0
	for line := range bytes.Lines(content) {
		starts = append(starts, offset)
		offset += len(line)
	}
	starts = append(starts, offset)

	out := bytes.Clone(content)
	for _, it := range items {
		if !boxes[it.SourceLine] {
			return nil, fmt.Errorf("%w: line %d of %q does not hold a checkbox", specprovider.ErrItemMoved, it.SourceLine, it.Title)
		}
		start := starts[it.SourceLine-1]
		marker := markerOffset(out[start:starts[it.SourceLine]])
		if marker < 0 {
			return nil, fmt.Errorf("%w: line %d has no checkbox marker", specprovider.ErrItemMoved, it.SourceLine)
		}
		at := start + marker
		switch done := it.Status == string(StatusDone); {
		case done && out[at] == ' ':
			out[at] = 'x'
		case !done && (out[at] == 'x' || out[at] == 'X'):
			out[at] = ' '
		}
	}
	return out, nil
}

// markerOffset is the offset of the marker character (" ", "x" or "X") of
// the checkbox on line, after its indentation and "- [" or "* [";
// -1 when line holds no checkbox.
func markerOffset(line []byte) int {
	text := string(line)
	indent := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	if !isCheckbox(strings.TrimSpace(text)) {
		return -1
	}
	return indent + len("- [")
}
