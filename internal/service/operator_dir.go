package service

import (
	"errors"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// errNoOperatorDir is returned when an operator directory is not configured.
var errNoOperatorDir = errors.New("directory not configured")

// operatorDir is a directory the operator configured (the knowledge content
// root, the benchmark datasets directory: KI-105, KI-107). Users name files
// below it; names are kept relative to it and resolved inside it through
// workspacefs (os.Root): no symlink leads out of it, a FIFO never blocks,
// only regular files are read.
type operatorDir struct {
	path  string   // absolute, as configured; "" when not configured
	paths []string // path and its resolved form: an absolute input may name either
}

func newOperatorDir(dir string) operatorDir {
	if dir == "" {
		return operatorDir{}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	d := operatorDir{path: abs, paths: []string{abs}}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		d.paths = append(d.paths, resolved)
	}
	return d
}

// relative returns name as a clean slash path relative to the directory: a
// relative path that stays below it, or an absolute path that lies inside it
// (lexically, as configured or resolved). ok is false for anything else.
func (d operatorDir) relative(name string) (rel string, ok bool) {
	if filepath.IsAbs(name) {
		clean := filepath.Clean(name)
		for _, dir := range d.paths {
			if r, err := filepath.Rel(dir, clean); err == nil && filepath.IsLocal(r) {
				return filepath.ToSlash(r), true
			}
		}
		return "", false
	}
	if !filepath.IsLocal(name) {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(name)), true
}

// open opens the directory.
func (d operatorDir) open() (*workspacefs.Root, error) {
	if d.path == "" {
		return nil, errNoOperatorDir
	}
	return workspacefs.OpenOperatorDir(d.path)
}

// abs returns the absolute path of rel (a result of relative) below the
// directory as configured.
func (d operatorDir) abs(rel string) string {
	return filepath.Join(d.path, filepath.FromSlash(rel))
}
