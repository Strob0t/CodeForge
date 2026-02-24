package markdownspec

import (
	"fmt"
	"strings"
)

// RenderMarkdown converts a flat list of SpecItems back into markdown text.
// The output preserves heading hierarchy, checkbox syntax, and descriptions.
func RenderMarkdown(items []SpecItem) []byte {
	var b strings.Builder
	prevLevel := ItemLevel("")

	for i, item := range items {
		// Add a blank line between items (except at the start).
		if i > 0 {
			// Extra blank line before headings for readability.
			if isHeading(item.Level) || isHeading(prevLevel) {
				b.WriteString("\n")
			}
		}

		switch item.Level {
		case LevelH1:
			fmt.Fprintf(&b, "# %s\n", item.Title)
		case LevelH2:
			fmt.Fprintf(&b, "## %s\n", item.Title)
		case LevelH3:
			fmt.Fprintf(&b, "### %s\n", item.Title)
		case LevelCheckbox:
			marker := "[ ]"
			if item.Status == StatusDone {
				marker = "[x]"
			}
			fmt.Fprintf(&b, "- %s %s\n", marker, item.Title)
		case LevelListItem:
			fmt.Fprintf(&b, "- %s\n", item.Title)
		}

		// Append description if present.
		if item.Description != "" {
			for _, line := range strings.Split(item.Description, "\n") {
				if line == "" {
					b.WriteString("\n")
				} else {
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
		}

		prevLevel = item.Level
	}

	return []byte(b.String())
}

// isHeading returns true for heading-level items.
func isHeading(level ItemLevel) bool {
	return level == LevelH1 || level == LevelH2 || level == LevelH3
}
