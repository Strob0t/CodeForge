package policy

import (
	"path"
	"slices"
	"strings"
)

// Commands whose effect cannot be derived from their word list: they run
// code given as arguments, a file or stdin, change what later words or
// commands execute, or set variables that later commands read. A command
// containing one of them is opaque.
var opaqueExecutables = map[string]bool{
	// Shells run their arguments or stdin as a script.
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "mksh": true,
	"ash": true, "fish": true, "csh": true, "tcsh": true, "busybox": true,
	"pwsh": true, "powershell": true, "tclsh": true, "wish": true,
	// Builtins and keywords that execute strings, redefine commands or
	// evaluate variable values as arithmetic (which runs $(...) they contain).
	"eval": true, "source": true, ".": true, "alias": true, "hash": true, "trap": true,
	"enable": true, "function": true, "coproc": true, "let": true,
	// Builtins that set (and export) variables read by later commands, e.g.
	// PATH; mapfile also runs -C callbacks.
	"export": true, "declare": true, "typeset": true, "local": true, "readonly": true,
	"read": true, "mapfile": true, "readarray": true, "getopts": true,
	// Wrappers whose wrapped command or environment is not analysed: they
	// run a command given in their operands or a shell string.
	"builtin": true, "exec": true, "stdbuf": true, "setsid": true,
	"sudo": true, "doas": true, "su": true, "watch": true, "strace": true,
	"chroot": true, "unshare": true, "flock": true, "parallel": true,
	"ionice": true, "chrt": true, "taskset": true, "numactl": true, "prlimit": true, "setpriv": true,
	"runuser": true, "nsenter": true, "setarch": true, "sg": true, "pkexec": true, "script": true,
	"unbuffer": true, "chronic": true, "ifne": true, "systemd-run": true, "fakeroot": true, "faketime": true,
	"valgrind": true, "ltrace": true, "firejail": true, "bwrap": true, "xvfb-run": true,
	"dbus-run-session": true, "run-parts": true, "at": true, "batch": true, "tmux": true, "screen": true,
	// Package runners that download and run programs.
	"npx": true, "pnpx": true, "bunx": true, "uvx": true,
	// Programs with shell escapes in their command language.
	"sqlite3": true, "psql": true, "mysql": true, "gdb": true,
	"vi": true, "vim": true, "nvim": true, "view": true, "ex": true, "emacs": true,
}

// Shell keywords that may precede a command in the same simple command.
var leadingKeywords = map[string]bool{
	"!": true, "{": true, "}": true, "if": true, "then": true, "else": true,
	"elif": true, "fi": true, "do": true, "done": true, "while": true, "until": true,
}

// interpreter describes the options of a script interpreter: short option
// letters and long options that take program text, and options that only
// print information and exit.
type interpreter struct {
	short string
	long  []string
	info  []string
}

var interpreters = map[string]interpreter{
	"python":  {short: "c", info: []string{"--version", "-V", "--help", "-h"}},
	"pypy":    {short: "c", info: []string{"--version", "-V", "--help", "-h"}},
	"perl":    {short: "eE", info: []string{"--version", "-v", "-V", "--help", "-h"}},
	"ruby":    {short: "e", info: []string{"--version", "--help", "-h"}},
	"node":    {short: "ep", long: []string{"--eval", "--print"}, info: []string{"--version", "-v", "--help", "-h"}},
	"nodejs":  {short: "ep", long: []string{"--eval", "--print"}, info: []string{"--version", "-v", "--help", "-h"}},
	"bun":     {short: "ep", long: []string{"--eval", "--print"}, info: []string{"--version", "-v", "--help", "-h"}},
	"php":     {short: "r", info: []string{"--version", "-v", "--help", "-h"}},
	"lua":     {short: "e", info: []string{"-v"}},
	"julia":   {short: "e", info: []string{"--version", "-v", "--help", "-h"}},
	"Rscript": {short: "e", info: []string{"--version", "--help"}},
	"expect":  {short: "c", info: []string{"-v"}},
}

// interpreterFor returns the interpreter entry for an executable name such
// as "python3.12".
func interpreterFor(name string) (interpreter, bool) {
	it, ok := interpreters[strings.TrimRight(name, "0123456789.")]
	return it, ok
}

// classifySimpleCommand strips leading keywords and variable assignments,
// unwraps wrappers and the executable path from the words of one simple
// command. It reports opaque when the executable is not a literal word, an
// assignment is not accepted (leadingAssignments) or has no command, or the
// command runs code that the word list does not show.
func classifySimpleCommand(words []string, dynamic []bool) (seg []string, opaque bool) {
	i := 0
	for i < len(words) && leadingKeywords[words[i]] {
		i++
	}
	words, dynamic = words[i:], dynamic[i:]
	if len(words) == 0 {
		return nil, false
	}
	// Assignments before a command set its environment (KI-128). Without a
	// command they set shell variables for the rest of the command line,
	// which acts on the shell itself and on every later command.
	n, accepted := leadingAssignments(words, dynamic)
	if !accepted || n == len(words) {
		return nil, true
	}
	words, dynamic = words[n:], dynamic[n:]
	setsEnv := n > 0 // the command runs with assigned variables

	unknownArgs := false // arguments are appended at run time (xargs)
	for {
		exe := words[0]
		// An assignment after a wrapper is one after the time keyword, but
		// the name of the program that nice, timeout and the others run:
		// fail closed rather than guess which.
		if dynamic[0] || !isLiteralWord(exe) || isAssignment(exe) {
			return nil, true
		}
		base := path.Base(exe)
		unwrap, isWrapper := wrappers[base]
		// xargs appends its input to a wrapper's operands, where it becomes
		// the command the wrapper runs (or env's assignments).
		if isWrapper && unknownArgs {
			return nil, true
		}
		if !isWrapper || isCommandLookup(base, words[1:]) {
			break
		}
		start, ok := unwrap(words[1:])
		if !ok || slices.Contains(dynamic[1:1+start], true) {
			return nil, true
		}
		// Counts an -u operand with '=' as well (fail closed).
		setsEnv = setsEnv || (base == "env" && slices.ContainsFunc(words[1:1+start], isAssignment))
		if base == "xargs" {
			unknownArgs = true
			if 1+start == len(words) {
				return []string{"echo"}, false // xargs runs echo by default
			}
		}
		if 1+start == len(words) {
			if base == "env" {
				break // env without a command prints the environment
			}
			return nil, false // wrapper without a command runs nothing
		}
		words, dynamic = words[1+start:], dynamic[1+start:]
	}

	name := path.Base(words[0])
	args, argsDynamic := words[1:], dynamic[1:]
	switch {
	case opaqueExecutables[name], runsInlineCode(name, args), runsArgumentCode(name, args), evaluatesVariables(name, args):
		return nil, true
	case setsEnv && (name == "make" || name == "gmake"):
		// GNU make turns every environment variable into a make variable,
		// which overrides built-in defaults (RM, CC) and ?= assignments that
		// name the programs its recipes run.
		return nil, true
	case isArgumentSensitive(name) && (unknownArgs || slices.Contains(argsDynamic, true)):
		// Unknown arguments could add the options that run code.
		return nil, true
	}
	seg = make([]string, 0, len(words))
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

// isAssignment reports whether w is a variable assignment (NAME=value,
// NAME+=value or NAME[i]=value).
func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	name := strings.TrimSuffix(w[:eq], "+")
	if i := strings.IndexByte(name, '['); i > 0 {
		name = name[:i]
	}
	return isVariableName(name)
}

func isVariableName(s string) bool {
	if s == "" || !isLetter(s[0]) && s[0] != '_' {
		return false
	}
	for j := 1; j < len(s); j++ {
		if !isLetter(s[j]) && s[j] != '_' && (s[j] < '0' || s[j] > '9') {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// wrappers maps commands that run another command given in their arguments
// to a function returning the index where that command starts. ok is false
// when an option is not understood.
var wrappers = map[string]func(args []string) (start int, ok bool){
	"time":    unwrapFlags("-p"),
	"nohup":   unwrapFlags(),
	"command": unwrapFlags("-p"),
	"nice":    unwrapNice,
	"timeout": unwrapTimeout,
	"xargs":   unwrapXargs,
	"env":     unwrapEnv,
}

// isCommandLookup reports whether `command -v`/`-V` only looks a name up.
func isCommandLookup(name string, args []string) bool {
	return name == "command" && len(args) > 0 && (args[0] == "-v" || args[0] == "-V")
}

// unwrapFlags returns an unwrapper for a wrapper that takes only the given
// value-less flags (and "--"); any other option fails closed, e.g. the -o
// FILE of /usr/bin/time would otherwise look like the wrapped command.
func unwrapFlags(flags ...string) func(args []string) (int, bool) {
	return func(args []string) (int, bool) {
		for i, a := range args {
			switch {
			case a == "--":
				return i + 1, true
			case slices.Contains(flags, a):
			case strings.HasPrefix(a, "-"):
				return 0, false
			default:
				return i, true
			}
		}
		return len(args), true
	}
}

func unwrapNice(args []string) (int, bool) {
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			return i + 1, true
		case a == "-n" || a == "--adjustment":
			i += 2
		case strings.HasPrefix(a, "--adjustment="), strings.HasPrefix(a, "-n") && len(a) > 2, len(a) > 1 && a[0] == '-' && isDigits(a[1:]):
			i++
		case strings.HasPrefix(a, "-"):
			return 0, false
		default:
			return i, true
		}
	}
	return min(i, len(args)), true
}

// unwrapTimeout skips `timeout [OPTION]... DURATION`; a missing duration or
// command is a usage error and fails closed.
func unwrapTimeout(args []string) (int, bool) {
	i := 0
options:
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		a := args[i]
		switch {
		case a == "--":
			i++
			break options
		case a == "-s" || a == "-k" || a == "--signal" || a == "--kill-after":
			i += 2
		case strings.HasPrefix(a, "--signal="), strings.HasPrefix(a, "--kill-after="),
			(strings.HasPrefix(a, "-s") || strings.HasPrefix(a, "-k")) && len(a) > 2,
			a == "--preserve-status", a == "--foreground", a == "-v", a == "--verbose":
			i++
		default:
			return 0, false
		}
	}
	if i+1 >= len(args) { // DURATION and COMMAND are required
		return 0, false
	}
	return i + 1, true
}

// unwrapXargs skips the options of xargs. Input lines become additional
// arguments of the command, which classifySimpleCommand treats as unknown.
// Like -e, -i and -l, the long options --eof, --replace and --max-lines of
// GNU xargs take a value only after '='. A replace string (-I, -i,
// --replace) in the command name fails closed: busybox xargs replaces it
// there as well.
func unwrapXargs(args []string) (int, bool) {
	noValue := []string{"-0", "--null", "-r", "--no-run-if-empty", "-t", "--verbose", "-x", "--exit",
		"-p", "--interactive", "-o", "--open-tty", "-e", "--eof", "-l", "--max-lines"}
	longValue := []string{"--max-args", "--max-procs", "--max-chars", "--delimiter", "--arg-file"}
	replace, replaces := "", false
	i := 0
options:
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i++
			break options
		case !strings.HasPrefix(a, "-"):
			break options
		case a == "-i", a == "--replace":
			replace, replaces = "{}", true
		case a == "-I" && i+1 < len(args):
			i++
			replace, replaces = args[i], true
		case len(a) > 2 && (a[1] == 'I' || a[1] == 'i'):
			replace, replaces = a[2:], true
		case strings.HasPrefix(a, "--replace="):
			replace, replaces = strings.TrimPrefix(a, "--replace="), true
		case slices.Contains(noValue, a), hasAnyPrefix(a, "--eof=", "--max-lines="):
		case len(a) == 2 && strings.IndexByte("nLPsdaE", a[1]) >= 0, slices.Contains(longValue, a):
			i++ // the value is the next word
		case len(a) > 2 && strings.IndexByte("nLPsdaEel", a[1]) >= 0,
			slices.ContainsFunc(longValue, func(l string) bool { return strings.HasPrefix(a, l+"=") }):
		default:
			return 0, false
		}
	}
	if i >= len(args) {
		return len(args), true
	}
	if replaces && strings.Contains(args[i], replace) {
		return 0, false
	}
	return i, true
}

// runsInlineCode reports whether an interpreter invocation executes program
// text from its arguments or from stdin instead of a script file. An
// invocation with only informational options (--version, --help) does not.
func runsInlineCode(name string, args []string) bool {
	it, ok := interpreterFor(name)
	if !ok {
		return false
	}
	if len(args) > 0 && !slices.ContainsFunc(args, func(a string) bool { return !slices.Contains(it.info, a) }) {
		return false
	}
	hasProgram := false
	for _, a := range args {
		switch {
		case a == "-":
			return true // program read from stdin
		case strings.HasPrefix(a, "--"):
			for _, l := range it.long {
				if a == l || strings.HasPrefix(a, l+"=") {
					return true
				}
			}
		case strings.HasPrefix(a, "-"):
			if shortFlagCluster(a[1:], it.short) {
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
		if !isLetter(c) {
			return false
		}
	}
	return false
}

// evaluatesVariables reports whether the command evaluates its operands as
// arithmetic, which expands $(...) held in variable values (integer
// comparisons in [[ ]]), or assigns a variable (printf -v).
func evaluatesVariables(name string, args []string) bool {
	switch name {
	case "[[":
		for _, a := range args {
			switch a {
			case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
				return true
			}
		}
	case "printf":
		return slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-v") })
	}
	return false
}
