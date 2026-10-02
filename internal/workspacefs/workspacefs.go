// Package workspacefs is how the Go Core reads and writes workspace files in
// its own process (KI-95).
//
// Workspaces are untrusted: agents (the worker's tool user) can create files,
// symlinks, hard links and FIFOs anywhere in a workspace and swap them at any
// time. The Go Core reads workspace files with its own rights (the file
// browser and editor, goal and spec discovery, context scoring, stack and
// gate detection, checkpoints), so a workspace path must never resolve
// outside the workspace, and a special file must never block a request.
//
// Every name is resolved by os.Root below a descriptor of the workspace
// directory, one component at a time without following a symlink out of it:
// absolute names, ".." above the workspace and symlinks that leave it (an
// absolute target, even one that names a place inside, or one that climbs
// above it) are refused with ErrLeavesWorkspace; a relative symlink inside
// the workspace is followed, at most 8 per name. The worker's helper
// (codeforge.workspace_fs) applies the same rules. Opens use O_NONBLOCK and
// file reads and writes take regular files only (ErrNotRegular), so a FIFO,
// socket or device is never used. Walks and directory listings never descend
// into a symlink. The workspace directory itself must not be a symlink: the
// tool user can replace its workspace (the tenant directory is
// group-writable).
//
// Hard links cannot be told apart from regular files; with
// fs.protected_hardlinks=1 (the default of systemd-based hosts) the tool user
// can only link files it owns or may read and write.
//
// The KI-77 hardening of internal/git is separate: git runs as a process.
package workspacefs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

var (
	// ErrLeavesWorkspace marks a name that resolves outside the workspace.
	ErrLeavesWorkspace = errors.New("path leaves the workspace")
	// ErrNotRegular marks a name that is not a regular file (a directory, FIFO, socket or device).
	ErrNotRegular = errors.New("not a regular file")
	// ErrTooLarge marks a file over the caller's size cap.
	ErrTooLarge = errors.New("file too large")
)

// rootEscapeMessage is the text of os.Root's unexported escape error
// (TestEscapeErrorIsRecognised pins it).
const rootEscapeMessage = "path escapes from parent"

// Root is an open workspace directory.
type Root struct {
	root *os.Root
	path string
}

// Open opens the workspace directory at path. The components above it are
// the operator's and may be symlinks; the workspace directory itself must
// not be one.
func Open(path string) (*Root, error) {
	clean := filepath.Clean(path)
	root, err := os.OpenRoot(clean)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	// Compare what was opened with what the path names now: a symlink, or a
	// directory swapped either way meanwhile, is refused.
	named, err := os.Lstat(clean)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if named.Mode()&fs.ModeSymlink != 0 || !os.SameFile(opened, named) {
		_ = root.Close()
		return nil, fmt.Errorf("%w: the workspace directory %s is a symlink", ErrLeavesWorkspace, clean)
	}
	return &Root{root: root, path: clean}, nil
}

// OpenBelow opens the directory path, which must lie below base (an
// operator directory such as the workspace root): it is resolved inside
// base like a name, so no symlink on the way leads out of base.
func OpenBelow(base, path string) (*Root, error) {
	base = filepath.Clean(base)
	rel, err := filepath.Rel(base, filepath.Clean(path))
	if err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("%w: %s", ErrLeavesWorkspace, path)
	}
	baseRoot, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer func() { _ = baseRoot.Close() }()
	root, err := baseRoot.OpenRoot(rel)
	if err != nil {
		return nil, check(rel, err)
	}
	return &Root{root: root, path: filepath.Join(base, rel)}, nil
}

// ReadFileAt reads the regular file name of the workspace at workspacePath
// (at most maxSize bytes): Open, ReadFile and Close in one call.
func ReadFileAt(workspacePath, name string, maxSize int64) ([]byte, error) {
	ws, err := Open(workspacePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ws.Close() }()
	data, _, err := ws.ReadFile(name, maxSize)
	return data, err
}

// StatAt describes what name resolves to in the workspace at workspacePath.
func StatAt(workspacePath, name string) (fs.FileInfo, error) {
	ws, err := Open(workspacePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ws.Close() }()
	return ws.Stat(name)
}

// Close releases the workspace directory.
func (r *Root) Close() error {
	return r.root.Close()
}

// Path is the workspace directory as opened.
func (r *Root) Path() string {
	return r.path
}

func isEscape(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		if err.Error() == rootEscapeMessage {
			return true
		}
	}
	return false
}

// check gives errors of os.Root a clear meaning: escapes become
// ErrLeavesWorkspace; opening a socket or a FIFO for writing without a
// reader (ENXIO) and opening a directory for writing (EISDIR) become
// ErrNotRegular.
func check(name string, err error) error {
	switch {
	case err == nil:
		return nil
	case isEscape(err):
		return fmt.Errorf("%w: %s", ErrLeavesWorkspace, name)
	case errors.Is(err, syscall.ENXIO), errors.Is(err, syscall.EISDIR):
		return fmt.Errorf("%w: %s", ErrNotRegular, name)
	default:
		return err
	}
}

// Stat describes what name resolves to (symlinks inside the workspace followed).
func (r *Root) Stat(name string) (fs.FileInfo, error) {
	info, err := r.root.Stat(name)
	return info, check(name, err)
}

// Lstat describes name itself (a final symlink is not followed).
func (r *Root) Lstat(name string) (fs.FileInfo, error) {
	info, err := r.root.Lstat(name)
	return info, check(name, err)
}

// OpenFile opens the regular file name for reading without blocking.
func (r *Root) OpenFile(name string) (*os.File, fs.FileInfo, error) {
	f, err := r.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, check(name, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%w: %s", ErrNotRegular, name)
	}
	return f, info, nil
}

// ReadFile returns the content of the regular file name, ErrTooLarge above maxSize bytes.
func (r *Root) ReadFile(name string, maxSize int64) ([]byte, fs.FileInfo, error) {
	f, info, err := r.OpenFile(name)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > maxSize {
		return nil, nil, fmt.Errorf("%w: %s (%d bytes, max %d)", ErrTooLarge, name, info.Size(), maxSize)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > maxSize { // grew after the stat
		return nil, nil, fmt.Errorf("%w: %s (more than %d bytes)", ErrTooLarge, name, maxSize)
	}
	return data, info, nil
}

// WriteFile writes data to name: an existing regular file is overwritten in
// place (owner and mode kept, symlinks inside the workspace followed), a
// missing one is created with perm (the umask applies).
func (r *Root) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f, err := r.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|syscall.O_NONBLOCK, perm)
	if err != nil {
		return check(name, err)
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%w: %s", ErrNotRegular, name)
	}
	if err == nil {
		err = f.Truncate(0)
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// MkdirAll creates the directory name and its missing parents inside the workspace.
func (r *Root) MkdirAll(name string, perm fs.FileMode) error {
	return check(name, r.root.MkdirAll(name, perm))
}

// RemoveAll removes name and what it contains; a symlink is removed, never followed.
func (r *Root) RemoveAll(name string) error {
	return check(name, r.root.RemoveAll(name))
}

// Rename moves oldName to newName inside the workspace; a symlink is moved, never followed.
func (r *Root) Rename(oldName, newName string) error {
	err := r.root.Rename(oldName, newName)
	if isEscape(err) {
		return fmt.Errorf("%w: %s -> %s", ErrLeavesWorkspace, oldName, newName)
	}
	return err
}

// ReadDir lists the directory name (sorted by name; entries describe
// symlinks as symlinks).
func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := r.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, check(name, err)
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(-1)
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, err
}

// WalkDir walks the tree below name like fs.WalkDir (paths relative to the
// workspace); it never descends into a symlink.
func (r *Root) WalkDir(name string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(r.FS(), name, fn)
}

// FS is the workspace as an fs.FS: Open takes regular files and
// directories only and never blocks.
func (r *Root) FS() fs.FS {
	return rootFS{r: r}
}

type rootFS struct {
	r *Root
}

func (f rootFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	file, err := f.r.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, check(name, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		_ = file.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrNotRegular}
	}
	return file, nil
}

func (f rootFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	return f.r.Stat(name)
}

func (f rootFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return f.r.ReadDir(name)
}
