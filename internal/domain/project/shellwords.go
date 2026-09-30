package project

import (
	"errors"
	"strings"
)

// splitCommand splits cmd into words like Python's shlex.split in POSIX mode,
// which the worker uses to run gate commands: words are separated by
// whitespace; single quotes keep everything literally; in double quotes a
// backslash escapes only `"` and `\`; outside quotes a backslash escapes the
// next character. There are no comments, variables or other expansions.
func splitCommand(cmd string) ([]string, error) {
	var (
		words   []string
		word    strings.Builder
		inWord  bool // a word was started (also by empty quotes)
		quote   rune // the open quote, 0 outside quotes
		escaped bool
	)
	for _, r := range cmd {
		switch {
		case escaped:
			if quote == '"' && r != '"' && r != '\\' {
				word.WriteRune('\\')
			}
			word.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				word.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	switch {
	case escaped:
		return nil, errors.New("no escaped character")
	case quote != 0:
		return nil, errors.New("no closing quotation")
	case inWord:
		words = append(words, word.String())
	}
	return words, nil
}
