package markdownspec

import (
	"bytes"
	"strings"
)

// ItemLevel describes the structural depth of a parsed item.
type ItemLevel string

const (
	LevelH1       ItemLevel = "h1"
	LevelH2       ItemLevel = "h2"
	LevelH3       ItemLevel = "h3"
	LevelCheckbox ItemLevel = "checkbox"
	LevelListItem ItemLevel = "list_item"
)

// ItemStatus describes the completion state of a parsed item.
type ItemStatus string

const (
	StatusTodo       ItemStatus = "todo"
	StatusDone       ItemStatus = "done"
	StatusInProgress ItemStatus = "in_progress"
)

// SpecItem represents a single parsed element from a markdown spec file.
type SpecItem struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      ItemStatus `json:"status"`
	SortOrder   int        `json:"sort_order"`
	Level       ItemLevel  `json:"level"`
	SourceLine  int        `json:"source_line"`
	Children    []SpecItem `json:"children,omitempty"`
}

// ParseMarkdown extracts structured items from markdown content.
// It recognizes headings (h1/h2/h3), checkbox items (- [ ] / - [x]),
// and plain list items (- text). Lines of any length are read (KI-203: a
// line over bufio.Scanner's 64 KiB limit used to end the parse silently).
// Lines inside fenced code blocks are code, never items, and a heading is
// indented at most three spaces (deeper indentation is code).
func ParseMarkdown(content []byte) []SpecItem {
	var items []SpecItem
	order := 0
	lineNum := 0
	var descLines []string
	var lastItem *SpecItem
	var fence codeFence

	flushDescription := func() {
		if lastItem != nil && len(descLines) > 0 {
			lastItem.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
			descLines = nil
		}
	}

	for raw := range bytes.Lines(content) {
		lineNum++
		line := lineText(raw)
		trimmed := strings.TrimSpace(line)

		// Fenced code: the fence lines and everything between them are
		// description, never items.
		if fence.update(line) {
			if lastItem != nil && trimmed != "" {
				descLines = append(descLines, trimmed)
			}
			continue
		}

		// Skip empty lines — they may separate description paragraphs.
		if trimmed == "" {
			if lastItem != nil && len(descLines) > 0 {
				descLines = append(descLines, "")
			}
			continue
		}

		// Headings: # H1, ## H2, ### H3
		if strings.HasPrefix(trimmed, "#") && headingIndent(line) {
			flushDescription()
			level, title := parseHeading(trimmed)
			if level == "" {
				// Not a recognized heading depth; treat as description.
				if lastItem != nil {
					descLines = append(descLines, trimmed)
				}
				continue
			}
			order++
			item := SpecItem{
				Title:      title,
				Status:     StatusTodo,
				SortOrder:  order,
				Level:      level,
				SourceLine: lineNum,
			}
			items = append(items, item)
			lastItem = &items[len(items)-1]
			descLines = nil
			continue
		}

		// Checkbox: - [ ] text or - [x] text (case-insensitive x)
		if isCheckbox(trimmed) {
			flushDescription()
			status, title := parseCheckbox(trimmed)
			order++
			item := SpecItem{
				Title:      title,
				Status:     status,
				SortOrder:  order,
				Level:      LevelCheckbox,
				SourceLine: lineNum,
			}
			items = append(items, item)
			lastItem = &items[len(items)-1]
			descLines = nil
			continue
		}

		// Plain list item: - text or * text
		if isListItem(trimmed) {
			flushDescription()
			title := parseListItem(trimmed)
			order++
			item := SpecItem{
				Title:      title,
				Status:     StatusTodo,
				SortOrder:  order,
				Level:      LevelListItem,
				SourceLine: lineNum,
			}
			items = append(items, item)
			lastItem = &items[len(items)-1]
			descLines = nil
			continue
		}

		// Anything else is treated as description for the last item.
		if lastItem != nil {
			descLines = append(descLines, trimmed)
		}
	}

	// Flush trailing description.
	flushDescription()

	return items
}

// lineText is a line of bytes.Lines without its line ending ("\n" or "\r\n").
func lineText(raw []byte) string {
	return strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
}

// leadingSpaces counts the spaces line starts with (a tab counts as four).
func leadingSpaces(line string) int {
	n := 0
	for _, c := range line {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// headingIndent reports whether line is indented little enough for a
// heading: four spaces or more make it code.
func headingIndent(line string) bool {
	return leadingSpaces(line) <= 3
}

// codeFence tracks whether the lines read so far are inside a fenced code
// block (CommonMark: a run of at least three backticks or tildes, indented
// at most three spaces, closed by a run of the same character at least as
// long with nothing after it; an unclosed fence runs to the end).
type codeFence struct {
	char byte // '`' or '~' while inside a fence, 0 outside
	size int  // length of the opening run
}

// update reads line and reports whether it is part of a fenced code block
// (an opening or closing fence line, or a line between them).
func (f *codeFence) update(line string) bool {
	if leadingSpaces(line) > 3 {
		return f.char != 0
	}
	rest := strings.TrimLeft(line, " ")
	if f.char != 0 {
		if n := fenceRun(rest, f.char); n >= f.size && strings.TrimSpace(rest[n:]) == "" {
			f.char, f.size = 0, 0
		}
		return true
	}
	for _, c := range []byte{'`', '~'} {
		n := fenceRun(rest, c)
		if n < 3 || (c == '`' && strings.Contains(rest[n:], "`")) {
			continue
		}
		f.char, f.size = c, n
		return true
	}
	return false
}

// fenceRun is the length of the run of c that s starts with.
func fenceRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// parseHeading extracts the level and title from a markdown heading line.
func parseHeading(line string) (level ItemLevel, title string) {
	if strings.HasPrefix(line, "### ") {
		return LevelH3, strings.TrimSpace(line[4:])
	}
	if strings.HasPrefix(line, "## ") {
		return LevelH2, strings.TrimSpace(line[3:])
	}
	if strings.HasPrefix(line, "# ") {
		return LevelH1, strings.TrimSpace(line[2:])
	}
	return "", ""
}

// isCheckbox returns true if the line is a markdown checkbox item.
func isCheckbox(line string) bool {
	return strings.HasPrefix(line, "- [ ] ") ||
		strings.HasPrefix(line, "- [x] ") ||
		strings.HasPrefix(line, "- [X] ") ||
		strings.HasPrefix(line, "* [ ] ") ||
		strings.HasPrefix(line, "* [x] ") ||
		strings.HasPrefix(line, "* [X] ")
}

// parseCheckbox extracts the status and title from a checkbox line.
func parseCheckbox(line string) (status ItemStatus, title string) {
	// Normalize: "- [x] Title" or "* [X] Title"
	inner := line[2:5] // "[ ]" or "[x]" or "[X]"
	title = strings.TrimSpace(line[6:])

	if inner == "[x]" || inner == "[X]" {
		return StatusDone, title
	}
	return StatusTodo, title
}

// isListItem returns true if the line is a plain markdown list item (not a checkbox).
func isListItem(line string) bool {
	if isCheckbox(line) {
		return false
	}
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

// parseListItem extracts the title from a plain list item.
func parseListItem(line string) string {
	return strings.TrimSpace(line[2:])
}
