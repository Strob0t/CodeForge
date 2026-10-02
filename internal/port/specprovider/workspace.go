package specprovider

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// Helpers the file-based providers share. Workspace files are read through
// workspacefs (KI-95): no symlink leads out of the workspace, walks never
// descend into a symlink, and only regular files are listed or read.

// HasDir reports whether dir is a directory of the workspace at workspacePath.
func HasDir(workspacePath, dir string) (bool, error) {
	info, err := workspacefs.StatAt(workspacePath, dir)
	if err != nil {
		if workspacefs.IsAbsent(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// ListFiles lists the regular files below dir (in the workspace at
// workspacePath) with one of the extensions exts (lower case, with the dot)
// as specs of the given format, titled by title.
func ListFiles(workspacePath, dir, format string, exts []string, title func(ws *workspacefs.Root, rel string) string) ([]Spec, error) {
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = ws.Close() }()

	var specs []Spec
	err = ws.WalkDir(dir, func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !slices.Contains(exts, strings.ToLower(path.Ext(rel))) {
			return nil
		}
		if info, statErr := ws.Stat(rel); statErr != nil || !info.Mode().IsRegular() {
			return nil
		}
		specs = append(specs, Spec{Path: rel, Format: format, Title: title(ws, rel)})
		return nil
	})
	if err != nil {
		if workspacefs.IsAbsent(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk %s/: %w", dir, err)
	}
	return specs, nil
}

// ReadFile reads the spec file specPath of the workspace (at most MaxSpecBytes).
func ReadFile(workspacePath, specPath string) ([]byte, error) {
	return workspacefs.ReadFileAt(workspacePath, specPath, MaxSpecBytes)
}

// FileBaseName is the file name of name without its extension.
func FileBaseName(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}
