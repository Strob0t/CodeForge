package policy

import "strings"

// Leading variable assignments (NAME=value cmd) and the operands of
// env NAME=value cmd set the environment of the command they precede, and
// of every program it starts. The command is checked as usual; an
// assignment is accepted only when its value is a plain literal and its
// variable cannot make the command run code that its word list does not
// show (KI-128). The listed variables
//   - change how the shell parses, looks up or runs commands, or where cd
//     goes;
//   - point a program at a home, configuration or toolchain directory or
//     file, whose content can name programs to run (git core.pager, npm
//     script-shell, kubectl exec plugins, ssh ProxyCommand, ...);
//   - make the dynamic loader or the C library load code or data from a
//     chosen place (glibc ignores these in setuid programs for that reason);
//   - carry code or command-line options that a runtime or tool reads
//     (options can name programs: GOFLAGS -toolexec, NODE_OPTIONS --require,
//     CFLAGS -wrapper, MAKEFLAGS --eval, TAR_OPTIONS --to-command);
//   - name a program that tools start (pager, editor, compiler, ssh);
//   - configure, through the environment, a tool that reads any of its
//     options there (git, npm, yarn, cargo, rustup, the go command).
//
// Module search paths (PYTHONPATH, NODE_PATH, PERL5LIB, RUBYLIB, CLASSPATH,
// GOPATH) stay allowed: they choose the directories a runtime imports
// modules from by name, as the working directory already does for
// `python -m` and project-local packages, and the tools that read them run
// workspace code anyway. A command allow list for an interpreted tool trusts
// the modules it can import, as it trusts the build files of a build tool.
//
// An entry ending in '*' is a prefix, one starting with '*' a suffix, and
// one in '*...*' a part of the name. Names are compared in upper case, so a
// lower-case spelling is refused as well (fail closed).
var deniedEnvVariables = []string{
	// The shell.
	"PATH", "IFS", "ENV", "SHELL", "SHELLOPTS", "BASH*", "CDPATH", "PWD", "OLDPWD", "DIRSTACK",
	"GLOBIGNORE", "EXECIGNORE", "POSIXLY_CORRECT", "PS0", "PS1", "PS2", "PS3", "PS4", "PROMPT_COMMAND",
	"TEXTDOMAIN", "TEXTDOMAINDIR",
	// Home, configuration and toolchain locations.
	"*HOME", "XDG_*", "*CONFIG*", "PHPRC", "PHP_INI_SCAN_DIR", "WGETRC",
	// The dynamic loader and the C library.
	"LD_*", "DYLD_*", "GCONV_PATH", "GETCONF_DIR", "GLIBC_TUNABLES", "LOCPATH", "NLSPATH", "MALLOC_TRACE",
	// Code and options for runtimes and tools.
	"*FLAGS*", "*OPT", "*OPTS", "*OPTIONS", "PYTHONSTARTUP", "PYTHONINSPECT", "PYTHONWARNINGS",
	"PYTHONBREAKPOINT", "PERL5DB", "MAKEFILES", "MAKEOVERRIDES",
	// Programs that tools start.
	"*PAGER", "*EDITOR", "VISUAL", "BROWSER", "*ASKPASS", "*RSH", "*_SSH", "*WRAPPER", "RSYNC_CONNECT_PROG",
	"LESS*", "CC", "CXX", "CPP", "FC", "AR", "AS", "LD", "RUSTC", "RUSTDOC",
	// Tools configured through the environment.
	"GIT_*", "NPM_CONFIG_*", "YARN_*", "CARGO_*", "RUSTUP_*",
	"GOENV", "GOROOT", "GOTOOLCHAIN", "GOAUTH", "GOPROXY", "GONOPROXY", "GOPRIVATE", "GOSUMDB", "GONOSUMDB",
	"GONOSUMCHECK", "GOINSECURE", "GOVCS",
}

// deniedEnvVariable reports whether an assignment to name keeps a command
// opaque (deniedEnvVariables).
func deniedEnvVariable(name string) bool {
	name = strings.ToUpper(name)
	for _, entry := range deniedEnvVariables {
		prefix, isPrefix := strings.CutSuffix(entry, "*")
		suffix, isSuffix := strings.CutPrefix(entry, "*")
		switch {
		case isPrefix && isSuffix:
			if strings.Contains(name, prefix[1:]) {
				return true
			}
		case isPrefix:
			if strings.HasPrefix(name, prefix) {
				return true
			}
		case isSuffix:
			if strings.HasSuffix(name, suffix) {
				return true
			}
		case name == entry:
			return true
		}
	}
	return false
}

// acceptedAssignment reports whether a NAME=value word may set the
// environment of a command: NAME is a variable name (not NAME+= or
// NAME[i]=) that deniedEnvVariable does not refuse. That the value is a
// plain literal is the word's dynamic flag, which the caller checks.
func acceptedAssignment(w string) bool {
	name, _, ok := strings.Cut(w, "=")
	return ok && isVariableName(name) && !deniedEnvVariable(name)
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
