package markdownspec

import (
	"strconv"
	"strings"
	"testing"
)

// itemSummary renders items as "line:level:status:title" for comparisons.
func itemSummary(items []SpecItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, strings.Join([]string{strconv.Itoa(it.SourceLine), string(it.Level), string(it.Status), it.Title}, ":"))
	}
	return out
}

// KI-203: lines inside fenced code blocks are code, never headings or
// items; headings are indented at most three spaces (deeper is code).
func TestParseMarkdown_CodeFencesAndIndentedCode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "backtick fence",
			content: "# Title\n```sh\n# not a heading\n- [ ] not an item\n```\n- [ ] item\n",
			want:    []string{"1:h1:todo:Title", "6:checkbox:todo:item"},
		},
		{
			name:    "tilde fence",
			content: "~~~\n## not a heading\n~~~\n## Heading\n",
			want:    []string{"4:h2:todo:Heading"},
		},
		{
			name:    "longer closing fence and a shorter one inside",
			content: "````md\n```\n# inside\n```\n````\n# after\n",
			want:    []string{"6:h1:todo:after"},
		},
		{
			name:    "tilde inside backtick fence does not close it",
			content: "```\n~~~\n# inside\n```\n# after\n",
			want:    []string{"5:h1:todo:after"},
		},
		{
			name:    "indented fence",
			content: "   ```\n# inside\n   ```\n- [x] done\n",
			want:    []string{"4:checkbox:done:done"},
		},
		{
			name:    "backtick info string with a backtick is no fence",
			content: "``` a`b\n# heading\n",
			want:    []string{"2:h1:todo:heading"},
		},
		{
			name:    "unclosed fence runs to the end",
			content: "- [ ] before\n```\n# code\n- [ ] code\n",
			want:    []string{"1:checkbox:todo:before"},
		},
		{
			name:    "heading indented four spaces is code",
			content: "    # code\n   # heading\n",
			want:    []string{"2:h1:todo:heading"},
		},
		{
			name:    "nested checkboxes keep their indentation",
			content: "- [ ] parent\n    - [x] child\n\t- [ ] tabbed\n",
			want:    []string{"1:checkbox:todo:parent", "2:checkbox:done:child", "3:checkbox:todo:tabbed"},
		},
		{
			name:    "CRLF line endings",
			content: "# Title\r\n- [x] done\r\n- [ ] open\r\n",
			want:    []string{"1:h1:todo:Title", "2:checkbox:done:done", "3:checkbox:todo:open"},
		},
		{
			name:    "no trailing newline",
			content: "- [ ] last",
			want:    []string{"1:checkbox:todo:last"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itemSummary(ParseMarkdown([]byte(tt.content)))
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("ParseMarkdown =\n  %v\nwant\n  %v", got, tt.want)
			}
		})
	}
}

// KI-203: a line over bufio.Scanner's 64 KiB limit stopped the scan
// silently and every item after it was lost. Lines of any length are read.
func TestParseMarkdown_LongLines(t *testing.T) {
	for _, size := range []int{64*1024 - 1, 64 * 1024, 64*1024 + 1, 900 * 1024} {
		content := "- [ ] before\n" + strings.Repeat("a", size) + "\n- [x] after\n## Heading\n"
		got := itemSummary(ParseMarkdown([]byte(content)))
		want := []string{"1:checkbox:todo:before", "3:checkbox:done:after", "4:h2:todo:Heading"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("line of %d bytes: ParseMarkdown = %v, want %v", size, got, want)
		}
	}
}
