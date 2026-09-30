package policy

import (
	"path"
	"slices"
	"strings"
)

// shellCommand is the static analysis of a command line as the bash tool runs
// it (`bash -c <command>`). The analysis is deliberately conservative: when a
// construct could run code that is not visible as a plain word list, the
// whole command is marked opaque. An opaque command never satisfies an allow
// list and is denied by every command deny list (ADR-015).
type shellCommand struct {
	// segments holds one entry per simple command (split on ;, &, &&, |, ||,
	// |&, newline and parentheses). Each entry starts with the basename of the
	// executable, followed by its arguments after quote removal. Keywords,
	// redirections, comments and here-document bodies are dropped, and
	// wrappers such as timeout or xargs are replaced by the command they run.
	segments [][]string
	opaque   bool
}

// parseShellCommand splits cmd into simple commands and flags constructs that
// cannot be analysed statically.
func parseShellCommand(cmd string) shellCommand {
	p := shellParser{cmd: cmd}
	p.run()
	return shellCommand{segments: p.segments, opaque: p.opaque}
}

// wordRole tells endWord what the word being read is used for.
type wordRole int

const (
	roleArg            wordRole = iota
	roleRedirectTarget          // file of <, >, >>, &>, ...
	roleHereString              // word of <<<
	roleHeredoc                 // delimiter of <<
	roleHeredocStrip            // delimiter of <<- (leading tabs stripped)
)

// heredoc is a here-document whose body follows the current line.
type heredoc struct {
	delim  string
	quoted bool // any part of the delimiter was quoted: the body is not expanded
	strip  bool
}

type shellParser struct {
	cmd      string
	segments [][]string
	opaque   bool

	// Words of the simple command being read and whether each one contains
	// an expansion or an unquoted glob, so its value is not known statically.
	words   []string
	dynamic []bool

	word       strings.Builder
	inWord     bool // the current word has content or an opening quote
	wordDyn    bool // the current word contains a parameter expansion
	wordQuoted bool // part of the current word was quoted or escaped
	// Unquoted glob and brace characters seen in the current word.
	sawStar, sawOpenBracket, sawCloseBracket, sawOpenBrace, sawCloseBrace bool

	next     wordRole
	heredocs []heredoc // delimiters read on the current line
}

func (p *shellParser) add(c byte) {
	p.word.WriteByte(c)
	p.inWord = true
}

func (p *shellParser) resetWord() {
	p.word.Reset()
	p.inWord, p.wordDyn, p.wordQuoted = false, false, false
	p.sawStar, p.sawOpenBracket, p.sawCloseBracket, p.sawOpenBrace, p.sawCloseBrace = false, false, false, false, false
}

func (p *shellParser) endWord() {
	if !p.inWord {
		return
	}
	w, quoted := p.word.String(), p.wordQuoted
	dyn := p.wordDyn || p.sawStar || (p.sawOpenBracket && p.sawCloseBracket) || (p.sawOpenBrace && p.sawCloseBrace)
	p.resetWord()
	role := p.next
	p.next = roleArg
	switch role {
	case roleRedirectTarget:
		// bash opens network connections for /dev/tcp and /dev/udp
		// redirections; a computed target cannot be checked.
		clean := path.Clean(w)
		if dyn || strings.Contains(clean, "/dev/tcp/") || strings.Contains(clean, "/dev/udp/") {
			p.opaque = true
		}
	case roleHereString:
		// Data on stdin; expansions that run code are caught by the tokenizer.
	case roleHeredoc, roleHeredocStrip:
		p.heredocs = append(p.heredocs, heredoc{delim: w, quoted: quoted, strip: role == roleHeredocStrip})
	default:
		p.words = append(p.words, w)
		p.dynamic = append(p.dynamic, dyn)
	}
}

func (p *shellParser) endSegment() {
	p.endWord()
	p.next = roleArg
	words, dynamic := p.words, p.dynamic
	p.words, p.dynamic = nil, nil
	seg, opaque := classifySimpleCommand(words, dynamic)
	if opaque {
		p.opaque = true
		return
	}
	if len(seg) > 0 {
		p.segments = append(p.segments, seg)
	}
}

// run tokenizes the command following the bash rules that matter for
// splitting: quoting (single, double, ANSI-C $'...'), backslash escapes,
// comments, operators, redirections and here-documents.
func (p *shellParser) run() {
	const (
		unquoted = iota
		single
		double
		ansiC
	)
	cmd := p.cmd
	state := unquoted
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		next := byteAt(cmd, i+1)

		switch state {
		case single:
			if c == '\'' {
				state = unquoted
			} else {
				p.add(c)
			}
			continue
		case ansiC:
			switch c {
			case '\\':
				p.add(c)
				if i+1 < len(cmd) {
					i++
					p.add(cmd[i])
				}
			case '\'':
				state = unquoted
			default:
				p.add(c)
			}
			continue
		}

		if runsExpansion(cmd, i) {
			p.opaque = true
		}
		if state == double {
			switch {
			case c == '"':
				state = unquoted
			case c == '\\' && strings.IndexByte("$`\"\\\n", next) >= 0:
				i++
				if next != '\n' {
					p.add(next)
				}
			default:
				if c == '$' {
					p.wordDyn = true
				}
				p.add(c)
			}
			continue
		}

		switch c {
		case '\'':
			state = single
			p.inWord, p.wordQuoted = true, true
		case '"':
			state = double
			p.inWord, p.wordQuoted = true, true
		case '$':
			// $'...' is ANSI-C quoting; its escapes are not decoded, so the
			// word counts as unknown.
			p.wordDyn = true
			p.add(c)
			if next == '\'' {
				state = ansiC
				p.wordQuoted = true
				i++
			}
		case '\\':
			if next != 0 {
				i++
				if next != '\n' {
					p.add(next)
					p.wordQuoted = true
				}
			}
		case '#':
			if p.inWord {
				p.add(c)
				continue
			}
			// A comment runs to the end of the line.
			if nl := strings.IndexByte(cmd[i:], '\n'); nl >= 0 {
				i += nl - 1
			} else {
				i = len(cmd)
			}
		case ' ', '\t':
			p.endWord()
		case '\n':
			p.endSegment()
			if len(p.heredocs) > 0 {
				i = p.readHeredocBodies(i+1) - 1
			}
		case '(':
			if next == '(' { // arithmetic command evaluates variable values
				p.opaque = true
			}
			p.endSegment()
		case ';', ')':
			p.endSegment()
		case '|':
			p.endSegment()
			if next == '|' || next == '&' {
				i++
			}
		case '&':
			switch next {
			case '&':
				p.endSegment()
				i++
			case '>': // &> and &>> redirect stdout and stderr
				p.endWord()
				i++
				if byteAt(cmd, i+1) == '>' {
					i++
				}
				p.next = roleRedirectTarget
			default: // background operator
				p.endSegment()
			}
		case '<', '>':
			if next == '(' { // process substitution
				p.opaque = true
			}
			i = p.redirection(i)
		case '*', '?':
			p.sawStar = true
			p.add(c)
		case '[', ']', '{', '}':
			p.sawOpenBracket = p.sawOpenBracket || c == '['
			p.sawCloseBracket = p.sawCloseBracket || c == ']'
			p.sawOpenBrace = p.sawOpenBrace || c == '{'
			p.sawCloseBrace = p.sawCloseBrace || c == '}'
			p.add(c)
		default:
			p.add(c)
		}
	}
	if state != unquoted || len(p.heredocs) > 0 {
		p.opaque = true
	}
	p.endSegment()
}

func byteAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// runsExpansion reports whether unquoted or double-quoted text at s[i]
// starts an expansion that runs code or evaluates variable values as code:
// command substitution (`...`, $(...), including arithmetic $((...))), the
// old arithmetic form $[...] and every ${...} other than a plain ${name}
// (substring offsets, array subscripts, @P and nested expansions all do).
func runsExpansion(s string, i int) bool {
	rest := s[i:]
	switch {
	case strings.HasPrefix(rest, "`"), strings.HasPrefix(rest, "$("), strings.HasPrefix(rest, "$["):
		return true
	case strings.HasPrefix(rest, "${"):
		end := strings.IndexByte(rest, '}')
		return end < 0 || !isPlainParameter(rest[2:end])
	}
	return false
}

// isPlainParameter reports whether name is a variable name, a positional
// parameter or a special parameter.
func isPlainParameter(name string) bool {
	if len(name) == 1 && strings.IndexByte("@*#?-$!", name[0]) >= 0 {
		return true
	}
	return isDigits(name) || isVariableName(name)
}

// redirection consumes a redirection operator (<, <<, <<-, <<<, <&, <>, >,
// >>, >&, >|) starting at cmd[i] and returns the index of its last byte. A
// preceding all-digit word is the file descriptor and is discarded; the role
// of the following word (file, here-string or here-document delimiter) is
// recorded. Only the exact operator forms are consumed so that a pipe or list
// operator right after a redirection still splits the command.
func (p *shellParser) redirection(i int) int {
	cmd := p.cmd
	if p.inWord && isDigits(p.word.String()) {
		p.resetWord()
	} else {
		p.endWord()
	}
	role := roleRedirectTarget
	switch {
	case cmd[i] == '<' && byteAt(cmd, i+1) == '<':
		i++
		switch byteAt(cmd, i+1) {
		case '<':
			i++
			role = roleHereString
		case '-':
			i++
			role = roleHeredocStrip
		default:
			role = roleHeredoc
		}
	case cmd[i] == '<' && (byteAt(cmd, i+1) == '&' || byteAt(cmd, i+1) == '>'):
		i++
	case cmd[i] == '>' && (byteAt(cmd, i+1) == '>' || byteAt(cmd, i+1) == '&' || byteAt(cmd, i+1) == '|'):
		i++
	}
	p.next = role
	return i
}

// readHeredocBodies skips the bodies of the here-documents opened on the
// line that ended just before start and returns the index of the first byte
// after the last delimiter line. Bodies are data: they are not split into
// commands, but a body with an unquoted delimiter is expanded by bash, so an
// expansion that runs code makes the command opaque. A missing delimiter
// makes the command opaque as well.
func (p *shellParser) readHeredocBodies(start int) int {
	cmd := p.cmd
	pos := start
	for _, h := range p.heredocs {
		for {
			if pos >= len(cmd) {
				p.opaque = true
				p.heredocs = nil
				return len(cmd)
			}
			lineEnd := len(cmd)
			if nl := strings.IndexByte(cmd[pos:], '\n'); nl >= 0 {
				lineEnd = pos + nl
			}
			line := cmd[pos:lineEnd]
			pos = lineEnd + 1
			check := line
			if h.strip {
				check = strings.TrimLeft(line, "\t")
			}
			if check == h.delim {
				break
			}
			if !h.quoted && lineRunsCode(line) {
				p.opaque = true
			}
		}
	}
	p.heredocs = nil
	return min(pos, len(cmd))
}

func lineRunsCode(line string) bool {
	for i := range line {
		if runsExpansion(line, i) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for j := 0; j < len(s); j++ {
		if s[j] < '0' || s[j] > '9' {
			return false
		}
	}
	return true
}

// allowedBy reports whether every simple command matches one of the
// patterns. Opaque and empty commands never match.
func (c shellCommand) allowedBy(patterns []string) bool {
	if c.opaque || len(c.segments) == 0 {
		return false
	}
	for _, seg := range c.segments {
		if !matchesAnyCommandPattern(patterns, seg, false) {
			return false
		}
	}
	return true
}

// deniedBy reports whether the command must be denied by a deny list: when
// it is opaque, empty, or any simple command matches one of the patterns.
// Deny matching ignores case so that it errs on the side of denying.
func (c shellCommand) deniedBy(patterns []string) bool {
	if c.opaque || len(c.segments) == 0 {
		return true
	}
	for _, seg := range c.segments {
		if matchesAnyCommandPattern(patterns, seg, true) {
			return true
		}
	}
	return false
}

// matchesAnyCommandPattern matches a simple command against command prefix
// patterns such as "git status": the executable basename and the following
// words must equal the pattern words.
func matchesAnyCommandPattern(patterns, seg []string, foldCase bool) bool {
	for _, pattern := range patterns {
		pw := strings.Fields(pattern)
		if len(pw) == 0 || len(pw) > len(seg) {
			continue
		}
		pw[0] = path.Base(pw[0])
		matched := true
		for j := range pw {
			if !wordEqual(pw[j], seg[j], foldCase) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func wordEqual(a, b string, foldCase bool) bool {
	if foldCase {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// CommandExecutables returns the executable basenames of all simple commands
// in cmd, deduplicated in order of appearance. It reports false when cmd is
// empty or cannot be analysed statically.
func CommandExecutables(cmd string) ([]string, bool) {
	c := parseShellCommand(cmd)
	if c.opaque || len(c.segments) == 0 {
		return nil, false
	}
	var exes []string
	for _, seg := range c.segments {
		if !slices.Contains(exes, seg[0]) {
			exes = append(exes, seg[0])
		}
	}
	return exes, true
}
