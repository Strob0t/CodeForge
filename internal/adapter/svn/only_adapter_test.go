package svn

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// programCall matches a process start whose program is svn or gh.
var programCall = regexp.MustCompile(`(?:exec\.Command(?:Context)?|execCommand|\.Command)\((?:ctx, )?"(svn|gh)"`)

// KI-117: the Go Core image carries svn for this adapter, which runs it
// hardened; no other package starts svn, and nothing starts gh (GitHub
// issues and pull requests go through the REST API).
func TestOnlyTheSVNAdapterRunsSVNAndNothingRunsGH(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	var found []string
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			data, err := os.ReadFile(path) //nolint:gosec // G304: the repository's own Go sources
			if err != nil {
				return err
			}
			for _, m := range programCall.FindAllStringSubmatch(string(data), -1) {
				inAdapter := filepath.Dir(path) == filepath.Join(root, "internal", "adapter", "svn")
				if m[1] == "gh" || !inAdapter {
					found = append(found, path+": "+m[0])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) > 0 {
		t.Fatalf("svn outside the SVN adapter or gh at all:\n%s", strings.Join(found, "\n"))
	}
	adapter, err := os.ReadFile("provider.go")
	if err != nil {
		t.Fatal(err)
	}
	if !programCall.MatchString(string(adapter)) {
		t.Fatal("the pattern no longer finds the adapter's own svn call: update it")
	}
}
