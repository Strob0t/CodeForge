package git

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Special files (KI-187). git opens some files of the workspace by name with
// a blocking open(): the ignore, attributes, mailmap and .gitmodules files,
// the files of .git and .git/info, refs, reflogs, packs and core.excludesFile.
// A FIFO there blocks git until something writes to it. OpenRepo refuses a
// FIFO, socket or device (or a symbolic link to one) at these places before
// any git command reads the working tree; the nested walk refuses the ignore
// and attributes files of every directory it enters. A FIFO .gitignore in a
// new directory is read by the listing of ignored directories that precedes
// the walk: the command deadline (deadline.go) ends that one.

// rootFilesGitOpens are the files of the worktree root git reads by name.
var rootFilesGitOpens = []string{".gitignore", ".gitattributes", ".mailmap", ".gitmodules"}

// isSpecial reports whether a file of mode m is a FIFO, socket or device.
func isSpecial(m fs.FileMode) bool {
	return m&(fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice|fs.ModeIrregular) != 0
}

// refuseSpecialFile refuses path (named rel) when it is a FIFO, socket or
// device, or a symbolic link to one. A missing file or a dangling link is
// fine: git skips a file it cannot open.
func refuseSpecialFile(path, rel string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", rel, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Stat(path) // stat never opens the target
		if err != nil {
			return nil //nolint:nilerr // git cannot open it either
		}
		info = target
	}
	if isSpecial(info.Mode()) {
		return unsafeRepo(rel + " is a FIFO, socket or device, which git would block on (KI-187)")
	}
	return nil
}

// refuseSpecialEntries refuses the special entries (refuseSpecialFile) of
// the directory dir (named rel); subdirectories are not entered.
func refuseSpecialEntries(dir, rel string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", rel, err)
	}
	for _, e := range entries {
		if err := refuseSpecialFile(filepath.Join(dir, e.Name()), rel+"/"+e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// refuseSpecialRootFiles refuses special files git opens by name in the
// worktree root.
func (r *Repo) refuseSpecialRootFiles() error {
	for _, name := range rootFilesGitOpens {
		if err := refuseSpecialFile(filepath.Join(r.Dir, name), name); err != nil {
			return err
		}
	}
	return nil
}

// checkExcludesFile refuses a core.excludesFile git would read outside the
// worktree or that is not a regular file there: git reads it with every
// ignore lookup. A missing file is fine. core.attributesFile needs no check:
// it is overridden on the command line (commonOverrides) and never read.
func (r *Repo) checkExcludesFile() error {
	values := r.config["core.excludesfile"]
	if len(values) == 0 || values[len(values)-1] == "" {
		return nil
	}
	value := values[len(values)-1]
	refuse := func(why string) error {
		return unsafeRepo(fmt.Sprintf("config key \"core.excludesfile\" (%s) %s; it must name a regular file inside the workspace (KI-187)", value, why))
	}
	if strings.HasPrefix(value, "~") || strings.HasPrefix(value, "%(") {
		return refuse("names a file outside the workspace")
	}
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Dir, path) // git runs in the worktree
	}
	rel, err := filepath.Rel(r.Dir, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return refuse("names a file outside the workspace")
	}
	root, err := os.OpenRoot(r.Dir)
	if err != nil {
		return fmt.Errorf("open workspace %s: %w", r.Dir, err)
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return refuse("cannot be checked inside the workspace: " + err.Error())
	case !info.Mode().IsRegular():
		return refuse("is not a regular file")
	}
	return nil
}
