package policy

import (
	"slices"
	"strings"
)

// Leading variable assignments (NAME=value cmd) and the operands of
// env NAME=value cmd set the environment of the command they precede, and
// of every program it starts. The command is checked as usual; an
// assignment is accepted only when its value is a plain literal and its
// variable is on the allow list envVariables (KI-128). Every other variable
// keeps the command opaque: the variables that make the shell, the loader
// or a tool run other code (PATH, LD_PRELOAD, GIT_*, NODE_OPTIONS,
// CC_<target>, PYTEST_PLUGINS, proxy and CA settings, ...) are too many to
// list. Names are compared exactly, as bash does: pythonpath is not
// PYTHONPATH. PYTHONWARNINGS is not listed: a warning category foo.Bar
// imports the module foo.
//
// The module search paths PYTHONPATH and NODE_PATH are accepted so that a
// command that sets one stays analysable and deny lists see it (KI-128:
// PYTHONPATH=src python -m unittest under a deny-list profile). They are
// not harmless: they make a runtime import modules by name from directories
// the command chooses, which the working directory does not do for most
// commands (PYTHONPATH=. python3 -m json.tool, pip list or black --check
// run ./sitecustomize.py; the same commands without it do not). So a simple
// command that sets one is never matched by an allow rule that names
// commands (a command allow list, an allow rule's sub-pattern, an
// Allow-Always rule): it falls through to the later rules and the
// profile's default (shellCommand.modulePath).

// envValue is the kind of value an allowed variable may take.
type envValue int

const (
	anyValue envValue = iota
	// localeName must not contain '/': glibc takes a locale name with a
	// slash for a path to locale data, and gettext builds catalogue paths
	// from it.
	localeName
)

// envVariables is the allow list of variables an assignment may set.
var envVariables = map[string]envValue{
	// Output and runtime switches.
	"CI": anyValue, "DEBUG": anyValue, "NO_COLOR": anyValue, "FORCE_COLOR": anyValue, "TERM": anyValue,
	"COLUMNS": anyValue, "LINES": anyValue, "TZ": anyValue, "NODE_ENV": anyValue,
	"RUST_BACKTRACE": anyValue, "RUST_LOG": anyValue,
	"PYTHONUNBUFFERED": anyValue, "PYTHONDONTWRITEBYTECODE": anyValue, "PYTHONHASHSEED": anyValue,
	"PYTHONIOENCODING": anyValue, "PYTHONUTF8": anyValue, "PYTHONFAULTHANDLER": anyValue,
	"GOOS": anyValue, "GOARCH": anyValue, "CGO_ENABLED": anyValue,
	// Locale (the glibc categories).
	"LANG": localeName, "LANGUAGE": localeName, "LC_ALL": localeName,
	"LC_CTYPE": localeName, "LC_NUMERIC": localeName, "LC_TIME": localeName, "LC_COLLATE": localeName,
	"LC_MONETARY": localeName, "LC_MESSAGES": localeName, "LC_PAPER": localeName, "LC_NAME": localeName,
	"LC_ADDRESS": localeName, "LC_TELEPHONE": localeName, "LC_MEASUREMENT": localeName,
	"LC_IDENTIFICATION": localeName,
	// Module search paths (moduleSearchPaths).
	"PYTHONPATH": anyValue, "NODE_PATH": anyValue,
}

// moduleSearchPaths are the variables of envVariables that choose where a
// runtime imports modules from: no allow rule matches a command that sets
// one.
var moduleSearchPaths = map[string]bool{"PYTHONPATH": true, "NODE_PATH": true}

// setsModuleSearchPath reports whether one of the words assigns a module
// search path.
func setsModuleSearchPath(words []string) bool {
	return slices.ContainsFunc(words, func(w string) bool {
		name, _, ok := strings.Cut(w, "=")
		return ok && moduleSearchPaths[name]
	})
}

// acceptedAssignment reports whether a NAME=value word may set the
// environment of a command: NAME is on the allow list (so the word is no
// NAME+= or NAME[i]=) and the value is of its kind. That the value is a
// plain literal is the word's dynamic flag, which the caller checks.
func acceptedAssignment(w string) bool {
	name, value, ok := strings.Cut(w, "=")
	kind, allowed := envVariables[name]
	return ok && allowed && (kind != localeName || !strings.Contains(value, "/"))
}

// leadingAssignments returns how many words at the start of a simple
// command are variable assignments and whether all of them are accepted.
func leadingAssignments(words []string, dynamic []bool) (n int, accepted bool) {
	accepted = true
	for n < len(words) && isAssignment(words[n]) {
		accepted = accepted && !dynamic[n] && acceptedAssignment(words[n])
		n++
	}
	return n, accepted
}

// unwrapEnv skips the options and NAME=value operands of env. An option
// other than -i, -u NAME and -- fails closed (-S splits a string into a
// command line, -C changes the directory), as does an operand that a
// leading assignment could not set; values with expansions are refused by
// the caller like other unwrapped words.
func unwrapEnv(args []string) (int, bool) {
	i := 0
options:
	for i < len(args) {
		switch a := args[i]; {
		case a == "--", a == "-": // "-" also clears the environment
			i++
			break options
		case a == "-i" || a == "--ignore-environment":
			i++
		case a == "-u" || a == "--unset":
			i += 2
		case strings.HasPrefix(a, "--unset="), strings.HasPrefix(a, "-u") && len(a) > 2:
			i++
		case strings.HasPrefix(a, "-"):
			return 0, false
		default:
			break options
		}
	}
	for i < len(args) && strings.Contains(args[i], "=") {
		if !acceptedAssignment(args[i]) {
			return 0, false
		}
		i++
	}
	return min(i, len(args)), true
}
