package policy

import (
	"path"
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
	// variable assignments and redirections are dropped.
	segments [][]string
	opaque   bool
}

// Commands that execute code given as arguments, a file or stdin, or that
// change what a later word executes. Their effect cannot be derived from the
// word list, so a command containing one of them is opaque.
var opaqueExecutables = map[string]bool{
	// Shells run their arguments or stdin as a script.
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "mksh": true,
	"ash": true, "fish": true, "csh": true, "tcsh": true, "busybox": true,
	// Builtins and keywords that execute strings, redefine commands or
	// evaluate variable values as arithmetic (which runs $(...) they contain).
	"eval": true, "source": true, ".": true, "alias": true, "hash": true, "trap": true,
	"enable": true, "function": true, "coproc": true, "let": true,
	// Wrappers that run the command given in their arguments.
	"env": true, "command": true, "builtin": true, "exec": true, "time": true,
	"nohup": true, "nice": true, "ionice": true, "timeout": true, "stdbuf": true,
	"setsid": true, "sudo": true, "doas": true, "su": true, "xargs": true,
	"watch": true, "strace": true, "chroot": true, "unshare": true, "flock": true,
}

// Shell keywords that may precede a command in the same simple command.
var leadingKeywords = map[string]bool{
	"!": true, "{": true, "}": true, "if": true, "then": true, "else": true,
	"elif": true, "fi": true, "do": true, "done": true, "while": true, "until": true,
}

// inlineCodeFlags lists, per interpreter, the short option letters that take
// program text as an argument, and the long options that do the same.
var inlineCodeFlags = map[string]struct {
	short string
	long  []string
}{
	"python": {short: "c"},
	"pypy":   {short: "c"},
	"perl":   {short: "eE"},
	"ruby":   {short: "e"},
	"node":   {short: "ep", long: []string{"--eval", "--print"}},
	"nodejs": {short: "ep", long: []string{"--eval", "--print"}},
	"bun":    {short: "ep", long: []string{"--eval", "--print"}},
	"php":    {short: "r"},
}

// parseShellCommand splits cmd into simple commands and flags constructs that
// cannot be analysed statically.
func parseShellCommand(cmd string) shellCommand {
	p := shellParser{}
	p.run(cmd)
	return shellCommand{segments: p.segments, opaque: p.opaque}
}

type shellParser struct {
	segments [][]string
	opaque   bool

	words    []string
	word     strings.Builder
	inWord   bool // true once the current word has content or an opening quote
	dropNext bool // the next word is a redirection target
}

func (p *shellParser) endWord() {
	if !p.inWord {
		return
	}
	w := p.word.String()
	p.word.Reset()
	p.inWord = false
	if p.dropNext {
		p.dropNext = false
		// bash opens network connections for /dev/tcp and /dev/udp
		// redirections; a computed target cannot be checked.
		clean := path.Clean(w)
		if strings.Contains(clean, "/dev/tcp/") || strings.Contains(clean, "/dev/udp/") || strings.ContainsAny(w, "$`") {
			p.opaque = true
		}
		return
	}
	p.words = append(p.words, w)
}

func (p *shellParser) endSegment() {
	p.endWord()
	p.dropNext = false
	words := p.words
	p.words = nil
	seg, opaque := simpleCommand(words)
	if opaque {
		p.opaque = true
		return
	}
	if len(seg) > 0 {
		p.segments = append(p.segments, seg)
	}
}

func (p *shellParser) add(c byte) {
	p.word.WriteByte(c)
	p.inWord = true
}

// run tokenizes cmd following the bash quoting rules that matter for
// splitting: single quotes, double quotes, backslash escapes, operators.
func (p *shellParser) run(cmd string) {
	const (
		unquoted = iota
		single
		double
	)
	state := unquoted
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		next := byte(0)
		if i+1 < len(cmd) {
			next = cmd[i+1]
		}

		if state == single {
			if c == '\'' {
				state = unquoted
			} else {
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
				p.add(c)
			}
			continue
		}

		switch c {
		case '\'':
			state = single
			p.inWord = true
		case '"':
			state = double
			p.inWord = true
		case '\\':
			if next != 0 {
				i++
				if next != '\n' {
					p.add(next)
				}
			}
		case ' ', '\t':
			p.endWord()
		case '(':
			if next == '(' { // arithmetic command evaluates variable values
				p.opaque = true
			}
			p.endSegment()
		case '\n', ';', ')':
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
				if i+1 < len(cmd) && cmd[i+1] == '>' {
					i++
				}
				p.dropNext = true
			default: // background operator
				p.endSegment()
			}
		case '<', '>':
			if next == '(' { // process substitution
				p.opaque = true
			}
			i = p.redirection(cmd, i)
		default:
			p.add(c)
		}
	}
	if state != unquoted {
		p.opaque = true
	}
	p.endSegment()
}

// runsExpansion reports whether unquoted or double-quoted text at cmd[i]
// starts an expansion that runs code: command substitution (`...` or $(...),
// which includes arithmetic expansion $((...))) or prompt expansion ${x@P}.
func runsExpansion(cmd string, i int) bool {
	rest := cmd[i:]
	return strings.HasPrefix(rest, "`") || strings.HasPrefix(rest, "$(") || strings.HasPrefix(rest, "@P}")
}

// redirection consumes a redirection operator (<, <<, <<<, <&, <>, >, >>,
// >&, >|) starting at cmd[i] and returns the index of its last byte. A
// preceding all-digit word is the file descriptor and is discarded; the
// following word is the target (or here-doc delimiter) and is discarded as
// well. Only the exact operator forms are consumed so that a pipe or list
// operator right after a redirection still splits the command.
func (p *shellParser) redirection(cmd string, i int) int {
	if p.inWord && isDigits(p.word.String()) {
		p.word.Reset()
		p.inWord = false
	} else {
		p.endWord()
	}
	at := func(j int) byte {
		if j < len(cmd) {
			return cmd[j]
		}
		return 0
	}
	switch {
	case cmd[i] == '<' && at(i+1) == '<':
		i++
		if at(i+1) == '<' {
			i++
		}
	case cmd[i] == '<' && (at(i+1) == '&' || at(i+1) == '>'):
		i++
	case cmd[i] == '>' && (at(i+1) == '>' || at(i+1) == '&' || at(i+1) == '|'):
		i++
	}
	p.dropNext = true
	return i
}

// simpleCommand strips leading keywords, assignments and the executable path
// from words. It reports opaque when the executable is not a literal word or
// runs code that the word list does not show.
func simpleCommand(words []string) (seg []string, opaque bool) {
	i := 0
	for i < len(words) && (leadingKeywords[words[i]] || isAssignment(words[i])) {
		i++
	}
	if i == len(words) {
		return nil, false
	}
	exe := words[i]
	if !isLiteralWord(exe) {
		return nil, true
	}
	name := path.Base(exe)
	args := words[i+1:]
	if opaqueExecutables[name] || runsInlineCode(name, args) || findRunsCommand(name, args) || evaluatesArithmetic(name, args) {
		return nil, true
	}
	seg = make([]string, 0, len(args)+1)
	seg = append(seg, name)
	return append(seg, args...), false
}

// isLiteralWord reports whether the shell uses w verbatim as a command name,
// i.e. it contains no expansion or glob characters.
func isLiteralWord(w string) bool {
	if w == "" {
		return false
	}
	if w == "[" || w == "[[" {
		return true
	}
	return !strings.ContainsAny(w, "$`*?[]{}")
}

func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	name := strings.TrimSuffix(w[:eq], "+")
	for j := 0; j < len(name); j++ {
		c := name[j]
		isAlpha := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isAlpha && (j == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return name != ""
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

// runsInlineCode reports whether an interpreter invocation executes program
// text from its arguments or from stdin instead of a script file.
func runsInlineCode(name string, args []string) bool {
	flags, ok := inlineCodeFlags[strings.TrimRight(name, "0123456789.")]
	if !ok {
		return false
	}
	hasProgram := false
	for _, a := range args {
		switch {
		case a == "-":
			return true // program read from stdin
		case strings.HasPrefix(a, "--"):
			for _, l := range flags.long {
				if a == l || strings.HasPrefix(a, l+"=") {
					return true
				}
			}
		case strings.HasPrefix(a, "-"):
			if shortFlagCluster(a[1:], flags.short) {
				return true
			}
		default:
			hasProgram = true
		}
	}
	return !hasProgram // no script argument: program read from stdin
}

// shortFlagCluster reports whether a cluster of single-letter options such
// as "Bc" or "ne" contains one of the letters in want before the first
// non-letter (option arguments may be attached, e.g. -c'code').
func shortFlagCluster(cluster, want string) bool {
	for j := 0; j < len(cluster); j++ {
		c := cluster[j]
		if strings.IndexByte(want, c) >= 0 {
			return true
		}
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return false
}

// evaluatesArithmetic reports whether the command evaluates its operands as
// arithmetic, which expands $(...) held in variable values: integer
// comparisons in [[ ]] and integer declarations.
func evaluatesArithmetic(name string, args []string) bool {
	switch name {
	case "[[":
		for _, a := range args {
			switch a {
			case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
				return true
			}
		}
	case "declare", "typeset", "local":
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && shortFlagCluster(a[1:], "i") {
				return true
			}
		}
	}
	return false
}

func findRunsCommand(name string, args []string) bool {
	if name != "find" {
		return false
	}
	for _, a := range args {
		switch a {
		case "-exec", "-execdir", "-ok", "-okdir":
			return true
		}
	}
	return false
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

// CommandExecutable returns the basename of the executable of the first
// simple command in cmd. It reports false when cmd is empty or cannot be
// analysed statically.
func CommandExecutable(cmd string) (string, bool) {
	c := parseShellCommand(cmd)
	if c.opaque || len(c.segments) == 0 {
		return "", false
	}
	return c.segments[0][0], true
}
