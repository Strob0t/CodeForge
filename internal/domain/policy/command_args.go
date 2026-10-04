package policy

import (
	"slices"
	"strings"
)

// argumentCode maps tools whose options or program arguments can make them
// run arbitrary commands to a check of their arguments. Such tools are also
// "argument sensitive": an argument with an unknown value (a variable, a glob
// or input appended by xargs) could add those options, so it makes the
// command opaque.
var argumentCode = map[string]func(args []string) bool{
	"find":   findRunsCode,
	"go":     goRunsCode,
	"git":    gitRunsCode,
	"awk":    awkRunsCode,
	"gawk":   awkRunsCode,
	"mawk":   awkRunsCode,
	"nawk":   awkRunsCode,
	"sed":    sedRunsCode,
	"gsed":   sedRunsCode,
	"make":   makeRunsCode,
	"gmake":  makeRunsCode,
	"tar":    tarRunsCode,
	"gtar":   tarRunsCode,
	"bsdtar": tarRunsCode,
	"rsync":  rsyncRunsCode,
	"man":    manRunsCode,
	"npm":    npmRunsCode,
	"pnpm":   npmRunsCode,
	"yarn":   npmRunsCode,
	"deno":   denoRunsCode,
}

// runsArgumentCode reports whether the arguments make the tool run code
// that is not visible as a command word.
func runsArgumentCode(name string, args []string) bool {
	check, ok := argumentCode[name]
	return ok && check(args)
}

// isArgumentSensitive reports whether the tool can run code depending on
// its arguments (interpreters and the tools in argumentCode).
func isArgumentSensitive(name string) bool {
	_, isInterpreter := interpreterFor(name)
	_, hasCheck := argumentCode[name]
	return isInterpreter || hasCheck
}

func hasAnyPrefix(a string, prefixes ...string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(a, p) })
}

// isShortCluster reports whether a is a cluster of single-letter options
// ("-xvf"), not a long option or a lone "-".
func isShortCluster(a string) bool {
	return len(a) > 1 && a[0] == '-' && a[1] != '-'
}

func findRunsCode(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return a == "-exec" || a == "-execdir" || a == "-ok" || a == "-okdir"
	})
}

// goRunsCode: -exec/-toolexec/-vettool run a program for each binary or
// tool, go generate runs the commands of //go:generate lines and go env -w
// persists such flags in GOFLAGS.
func goRunsCode(args []string) bool {
	sub := ""
	for _, a := range args {
		if sub == "" && !strings.HasPrefix(a, "-") {
			sub = a
		}
		for _, flag := range []string{"exec", "toolexec", "vettool"} {
			if a == "-"+flag || a == "--"+flag || hasAnyPrefix(a, "-"+flag+"=", "--"+flag+"=") {
				return true
			}
		}
	}
	return sub == "generate" || sub == "env" && slices.Contains(args, "-w")
}

// gitRunsCode covers configuration injected on the command line (-c,
// --config-env, git config) which can define aliases, pagers, ssh and diff
// commands, options that run programs or write files (--exec, -x, --ext-diff,
// --upload-pack, -O, --output, --template) and subcommands that run commands
// or configured tools.
func gitRunsCode(args []string) bool {
	sub := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if sub == "" {
			switch {
			case strings.HasPrefix(a, "-c"), hasAnyPrefix(a, "--config-env", "--exec-path"):
				return true
			case a == "-C" || a == "--git-dir" || a == "--work-tree" || a == "--namespace" || a == "--super-prefix":
				i++
			case !strings.HasPrefix(a, "-"):
				sub = a
			}
			continue
		}
		switch {
		case hasAnyPrefix(a, "--output", "--ext-diff", "--upload-pack", "--receive-pack", "--exec",
			"--extcmd", "--template", "--open-files-in-pager", "--config", "--tool", "-O"):
			return true
		case a == "-x", sub == "clone" && (a == "-u" || strings.HasPrefix(a, "-c")):
			return true
		}
	}
	switch sub {
	case "config", "filter-branch", "difftool", "mergetool", "bisect", "submodule",
		"daemon", "instaweb", "web--browse", "send-email", "archimport", "cvsimport":
		return true
	}
	return false
}

// awkRunsCode: awk programs run commands with system(), pipes and getline,
// gawk opens network connections via /inet and loads extensions with @load.
// Program files and extension options cannot be checked.
func awkRunsCode(args []string) bool {
	for _, a := range args {
		switch {
		case a == "-f" || a == "-E" || a == "-l" || a == "-i",
			hasAnyPrefix(a, "--file", "--exec", "--load", "--include"),
			isShortCluster(a) && a[1] == 'f':
			return true
		}
		for _, s := range []string{"system", "|", "getline", "/inet", "@"} {
			if strings.Contains(a, s) {
				return true
			}
		}
	}
	return false
}

// sedRunsCode: GNU sed runs commands with the e command and the e flag of
// s. Script files cannot be checked; --sandbox disables e, r and w.
func sedRunsCode(args []string) bool {
	var scripts, operands []string
	explicit, sandbox := false, false
	knownLong := []string{"--quiet", "--silent", "--debug", "--posix", "--regexp-extended", "--separate",
		"--unbuffered", "--null-data", "--zero-terminated", "--follow-symlinks", "--binary", "--help", "--version"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			operands = append(operands, args[i+1:]...)
			i = len(args)
		case a == "--sandbox":
			sandbox = true
		case a == "--expression":
			if i+1 >= len(args) {
				return true
			}
			i++
			scripts, explicit = append(scripts, args[i]), true
		case strings.HasPrefix(a, "--expression="):
			scripts, explicit = append(scripts, strings.TrimPrefix(a, "--expression=")), true
		case a == "--line-length":
			i++
		case slices.Contains(knownLong, a), hasAnyPrefix(a, "--in-place", "--line-length="):
		case strings.HasPrefix(a, "--"):
			return true
		case isShortCluster(a):
			script, consumed, ok := sedShortOptions(a[1:], args[i+1:])
			if !ok {
				return true
			}
			if script != nil {
				scripts, explicit = append(scripts, *script), true
			}
			i += consumed
		default:
			operands = append(operands, a)
		}
	}
	if !explicit {
		if len(operands) == 0 {
			return true
		}
		scripts = append(scripts, operands[0])
	}
	return !sandbox && slices.ContainsFunc(scripts, sedScriptRunsCode)
}

// sedShortOptions parses a cluster of short sed options. It returns the
// script given with -e (if any) and how many following arguments it used.
// In GNU sed "-ie" means -i with backup suffix "e".
func sedShortOptions(cluster string, rest []string) (script *string, consumed int, ok bool) {
	for j := 0; j < len(cluster); j++ {
		switch cluster[j] {
		case 'n', 'r', 'E', 's', 'u', 'z':
		case 'e':
			if j+1 < len(cluster) {
				s := cluster[j+1:]
				return &s, 0, true
			}
			if len(rest) == 0 {
				return nil, 0, false
			}
			return &rest[0], 1, true
		case 'i':
			return nil, 0, true // the rest of the cluster is the backup suffix
		case 'l':
			if j+1 == len(cluster) {
				return nil, 1, true
			}
			return nil, 0, true
		default: // includes -f (script file)
			return nil, 0, false
		}
	}
	return nil, 0, true
}

// sedScriptRunsCode reports whether a GNU sed script can execute commands
// (the e command or the e flag of s). Constructs it does not understand
// count as executing.
func sedScriptRunsCode(s string) bool {
	p := 0
	for {
		for p < len(s) && strings.IndexByte(" \t\n;", s[p]) >= 0 {
			p++
		}
		if p >= len(s) {
			return false
		}
		var ok bool
		if p, ok = skipSedAddresses(s, p); !ok {
			return true
		}
		for p < len(s) && strings.IndexByte(" \t!", s[p]) >= 0 {
			p++
		}
		if p >= len(s) {
			return true
		}
		c := s[p]
		p++
		switch c {
		case '{', '}', '=', 'd', 'D', 'g', 'G', 'h', 'H', 'n', 'N', 'p', 'P', 'x', 'z', 'F':
		case 's':
			var flags string
			if p, flags, ok = sedSubstitute(s, p); !ok || strings.ContainsRune(flags, 'e') {
				return true
			}
		case 'y':
			if p >= len(s) {
				return true
			}
			d := s[p]
			if p, ok = skipSedDelimited(s, p+1, d); !ok {
				return true
			}
			if p, ok = skipSedDelimited(s, p, d); !ok {
				return true
			}
		case 'a', 'i', 'c':
			p = sedTextEnd(s, p)
		case 'r', 'R', 'w', 'W', '#':
			p = sedLineEnd(s, p)
		case ':', 'b', 't', 'T', 'v':
			for p < len(s) && s[p] != ';' && s[p] != '\n' {
				p++
			}
		case 'q', 'Q', 'l', 'L':
			for p < len(s) && (s[p] == ' ' || (s[p] >= '0' && s[p] <= '9')) {
				p++
			}
		default: // includes the e command
			return true
		}
	}
}

// skipSedAddresses skips an optional address or address range.
func skipSedAddresses(s string, p int) (int, bool) {
	p, ok := skipSedAddress(s, p)
	if !ok {
		return p, false
	}
	if p < len(s) && s[p] == ',' {
		p++
		for p < len(s) && s[p] == ' ' {
			p++
		}
		return skipSedAddress(s, p)
	}
	return p, true
}

func skipSedAddress(s string, p int) (int, bool) {
	skipDigits := func() {
		for p < len(s) && s[p] >= '0' && s[p] <= '9' {
			p++
		}
	}
	if p >= len(s) {
		return p, true
	}
	switch c := s[p]; {
	case c >= '0' && c <= '9':
		skipDigits()
		if p < len(s) && s[p] == '~' {
			p++
			skipDigits()
		}
		return p, true
	case c == '$':
		return p + 1, true
	case c == '+' || c == '~':
		p++
		skipDigits()
		return p, true
	case c == '/' || c == '\\':
		d := byte('/')
		if c == '\\' {
			if p+1 >= len(s) {
				return p, false
			}
			d = s[p+1]
			p++
		}
		var ok bool
		if p, ok = skipSedDelimited(s, p+1, d); !ok {
			return p, false
		}
		for p < len(s) && (s[p] == 'I' || s[p] == 'M') {
			p++
		}
		return p, true
	}
	return p, true
}

// skipSedDelimited skips to just after the next unescaped delimiter d.
func skipSedDelimited(s string, p int, d byte) (int, bool) {
	for p < len(s) {
		switch s[p] {
		case '\\':
			p += 2
			continue
		case d:
			return p + 1, true
		case '\n':
			return p, false
		}
		p++
	}
	return p, false
}

// sedSubstitute skips the pattern and replacement of an s command and
// returns its flags; a w flag takes the rest of the line as file name.
func sedSubstitute(s string, p int) (next int, flags string, ok bool) {
	if p >= len(s) || s[p] == '\\' || s[p] == '\n' {
		return p, "", false
	}
	d := s[p]
	if p, ok = skipSedDelimited(s, p+1, d); !ok {
		return p, "", false
	}
	if p, ok = skipSedDelimited(s, p, d); !ok {
		return p, "", false
	}
	start := p
	for p < len(s) && strings.IndexByte(";\n}", s[p]) < 0 {
		if s[p] == 'w' {
			return sedLineEnd(s, p), s[start:p] + "w", true
		}
		p++
	}
	return p, s[start:p], true
}

func sedLineEnd(s string, p int) int {
	if nl := strings.IndexByte(s[p:], '\n'); nl >= 0 {
		return p + nl
	}
	return len(s)
}

// sedTextEnd skips the text of a, i or c, which continues on the next line
// while a line ends with a backslash.
func sedTextEnd(s string, p int) int {
	for {
		end := sedLineEnd(s, p)
		if end >= len(s) || s[end-1] != '\\' {
			return end
		}
		p = end + 1
	}
}

// makeRunsCode: --eval/-E add makefile text, command-line variables can
// override SHELL or the commands of recipes, and a makefile read from stdin
// cannot be checked.
func makeRunsCode(args []string) bool {
	for i, a := range args {
		switch {
		case strings.HasPrefix(a, "--eval"), a == "-E":
			return true
		case !strings.HasPrefix(a, "-") && strings.Contains(a, "="):
			return true
		case a == "-f" || a == "--file" || a == "--makefile":
			if i+1 < len(args) && (args[i+1] == "-" || strings.HasPrefix(args[i+1], "/dev/")) {
				return true
			}
		case hasAnyPrefix(a, "--file=-", "--makefile=-", "--file=/dev/", "--makefile=/dev/"),
			a == "-f-" || strings.HasPrefix(a, "-f/dev/"):
			return true
		}
	}
	return false
}

// tarRunsCode: options that pipe archive members or the archive through a
// program (--to-command, -I/--use-compress-program, -F/--info-script, ...).
func tarRunsCode(args []string) bool {
	for i, a := range args {
		switch {
		case hasAnyPrefix(a, "--to-command", "--checkpoint-action", "--use-compress-program",
			"--info-script", "--new-volume-script", "--rsh-command", "--rmt-command"):
			return true
		case isShortCluster(a) && strings.ContainsAny(a, "IF"):
			return true
		case i == 0 && !strings.HasPrefix(a, "-") && strings.ContainsAny(a, "IF"):
			return true // old-style bundled options, e.g. "tar xIf"
		}
	}
	return false
}

// rsyncRunsCode: -e/--rsh and --rsync-path name programs to run.
func rsyncRunsCode(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return hasAnyPrefix(a, "--rsh", "--rsync-path") || isShortCluster(a) && strings.ContainsRune(a, 'e')
	})
}

// manRunsCode: the pager and the HTML browser are programs to run.
func manRunsCode(args []string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		return hasAnyPrefix(a, "-P", "--pager", "-H", "--html")
	})
}

// npmRunsCode: npm exec/x download and run packages; --script-shell and
// --node-options change how scripts run; other config files cannot be
// checked.
func npmRunsCode(args []string) bool {
	for _, a := range args {
		if hasAnyPrefix(a, "--script-shell", "--node-options", "--userconfig", "--globalconfig") {
			return true
		}
	}
	valueOptions := []string{"--prefix", "-C", "--dir", "-w", "--workspace", "--registry", "--cache", "--cwd"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case slices.Contains(valueOptions, a):
			i++
		case !strings.HasPrefix(a, "-"):
			return a == "exec" || a == "x" || a == "dlx"
		}
	}
	return false
}

// denoRunsCode: deno eval runs program text.
func denoRunsCode(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a == "eval"
		}
	}
	return false
}
