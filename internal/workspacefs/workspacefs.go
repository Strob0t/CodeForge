// Package workspacefs is how the Go Core reads and writes workspace files in
// its own process (KI-95).
//
// Workspaces are untrusted: agents (the worker's tool user) can create files,
// symlinks, hard links and FIFOs anywhere in a workspace and swap them at any
// time. The Go Core reads workspace files with its own rights (the file
// browser and editor, goal and spec discovery, context scoring, stack and
// gate detection, checkpoints, delivery), so a workspace path must never
// resolve outside the workspace, and a special file must never block a
// request.
//
// Every name is resolved by os.Root below a descriptor of the workspace
// directory, one component at a time without following a symlink out of it:
// absolute names, ".." above the workspace and symlinks that leave it (an
// absolute target, even one that names a place inside, or one that climbs
// above it) are refused with ErrLeavesWorkspace; a relative symlink inside
// the workspace is followed, at most 8 per name. The worker's helper
// (codeforge.workspace_fs) applies the same rules. Opens never block and
// file reads and writes take regular files only (ErrNotRegular), so a FIFO,
// socket or device is never used. Walks and directory listings never descend
// into a symlink. The workspace directory itself must not be a symlink: the
// tool user can replace its workspace (the tenant directory is
// group-writable).
//
// Errors name the path inside the workspace, never the workspace's absolute
// path: they reach API responses.
//
// Hard links cannot be told apart from regular files; with
// fs.protected_hardlinks=1 (the default of systemd-based hosts) the tool user
// can only link files it owns or may read and write, and links never cross a
// mount, so only files on the workspace volume are reachable.
//
// The KI-77 hardening of internal/git is separate: git runs as a process.
package workspacefs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
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

// maxHeldDirs bounds the directory descriptors one WalkDir holds open; deeper
// directories are opened from the workspace root one at a time.
const maxHeldDirs = 64

// Root is an open workspace directory.
type Root struct {
	root *os.Root
	path string
}

// IsAbsent reports whether err means a name is not there for the workspace:
// missing, or refused because it leads out of it.
func IsAbsent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrLeavesWorkspace)
}

// openDir opens the directory dir with os.OpenRoot without blocking: os.Root
// opens without O_DIRECTORY and O_NONBLOCK, so a FIFO put in a directory's
// place would block it; "/." makes the kernel resolve the name as a
// directory first (a FIFO fails with ENOTDIR at once).
func openDir(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir + string(filepath.Separator) + ".")
	if err != nil {
		return nil, fmt.Errorf("open workspace: %w", stripPath(err))
	}
	return root, nil
}

// Open opens the workspace directory dir. The components above it are
// the operator's and may be symlinks; the workspace directory itself must
// not be one.
func Open(dir string) (*Root, error) {
	clean := filepath.Clean(dir)
	root, err := openDir(clean)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("open workspace: %w", stripPath(err))
	}
	// Compare what was opened with what the path names now: a symlink, or a
	// directory swapped either way meanwhile, is refused.
	named, err := os.Lstat(clean)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("open workspace: %w", stripPath(err))
	}
	if named.Mode()&fs.ModeSymlink != 0 || !os.SameFile(opened, named) {
		_ = root.Close()
		slog.Warn("workspace directory is a symlink, refused (KI-95)", "path", clean)
		return nil, fmt.Errorf("%w: the workspace directory is a symlink", ErrLeavesWorkspace)
	}
	return &Root{root: root, path: clean}, nil
}

// OpenBelow opens the directory dir, which must lie below base (an
// operator directory such as the workspace root or a tenant's area): it is
// resolved inside base like a name, so no symlink on the way leads out of
// base. Errors name dir relative to base.
func OpenBelow(base, dir string) (*Root, error) {
	base = filepath.Clean(base)
	rel, err := filepath.Rel(base, filepath.Clean(dir))
	if err != nil || !filepath.IsLocal(rel) {
		return nil, ErrLeavesWorkspace
	}
	baseRoot, err := openDir(base)
	if err != nil {
		return nil, err
	}
	defer func() { _ = baseRoot.Close() }()
	root, err := baseRoot.OpenRoot(rel + string(filepath.Separator) + ".") // never blocks on a FIFO (see openDir)
	if err != nil {
		return nil, check(rel, err)
	}
	return &Root{root: root, path: filepath.Join(base, rel)}, nil
}

// OpenOperatorDir opens a directory the operator configured (the knowledge
// content root, the benchmark datasets directory): its own path may contain
// symlinks, names below it are resolved like workspace names (no symlink out
// of it, never blocking, regular files only).
func OpenOperatorDir(dir string) (*Root, error) {
	clean := filepath.Clean(dir)
	root, err := openDir(clean)
	if err != nil {
		return nil, err
	}
	return &Root{root: root, path: clean}, nil
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

// Path is the workspace directory as opened (for server logs, not for responses).
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

// stripPath drops the path of a *fs.PathError or *os.LinkError, which for
// files opened through os.Root names the workspace's absolute path.
func stripPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}

// check gives errors of os.Root a clear meaning and names only the path
// inside the workspace: escapes become ErrLeavesWorkspace; opening a socket
// or a FIFO for writing without a reader, or a directory for writing,
// becomes ErrNotRegular.
func check(name string, err error) error {
	switch {
	case err == nil:
		return nil
	case isEscape(err):
		return fmt.Errorf("%w: %s", ErrLeavesWorkspace, name)
	case notRegularOpen(err):
		return fmt.Errorf("%w: %s", ErrNotRegular, name)
	default:
		return fmt.Errorf("%s: %w", name, stripPath(err))
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
	f, err := r.root.OpenFile(name, os.O_RDONLY|openNonBlock, 0)
	if err != nil {
		return nil, nil, check(name, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, check(name, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%w: %s", ErrNotRegular, name)
	}
	return f, info, nil
}

// ReadFile returns the content of the regular file name, ErrTooLarge above maxSize bytes.
func (r *Root) ReadFile(name string, maxSize int64) ([]byte, fs.FileInfo, error) {
	data, info, truncated, err := r.ReadFilePrefix(name, maxSize)
	if err != nil {
		return nil, nil, err
	}
	if truncated {
		return nil, nil, fmt.Errorf("%w: %s (%d bytes, max %d)", ErrTooLarge, name, info.Size(), maxSize)
	}
	return data, info, nil
}

// ReadFilePrefix returns at most maxSize bytes of the regular file name and
// whether the file is longer.
func (r *Root) ReadFilePrefix(name string, maxSize int64) (data []byte, info fs.FileInfo, truncated bool, err error) {
	f, info, err := r.OpenFile(name)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = f.Close() }()
	data, err = io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, nil, false, check(name, err)
	}
	if int64(len(data)) > maxSize {
		return data[:maxSize], info, true, nil
	}
	return data, info, false, nil
}

// WriteFile writes data to name: an existing regular file is overwritten in
// place (owner and mode kept, symlinks inside the workspace followed), a
// missing one is created with perm (the umask applies).
func (r *Root) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f, err := r.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|openNonBlock, perm)
	if err != nil {
		return check(name, err)
	}
	return writeChecked(name, f, data, true)
}

// CreateExclusive creates the new regular file name (it must not exist, a
// symlink included) and writes data to it.
func (r *Root) CreateExclusive(name string, data []byte, perm fs.FileMode) error {
	f, err := r.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|openNonBlock, perm)
	if err != nil {
		return check(name, err)
	}
	return writeChecked(name, f, data, false)
}

func writeChecked(name string, f *os.File, data []byte, truncate bool) error {
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%w: %s", ErrNotRegular, name)
	}
	if err == nil && truncate {
		err = f.Truncate(0)
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil && !errors.Is(err, ErrNotRegular) {
		return check(name, err)
	}
	return err
}

// Mkdir creates the directory name.
func (r *Root) Mkdir(name string, perm fs.FileMode) error {
	return check(name, r.root.Mkdir(name, perm))
}

// MkdirAll creates the directory name and its missing parents inside the workspace.
func (r *Root) MkdirAll(name string, perm fs.FileMode) error {
	return check(name, r.root.MkdirAll(name, perm))
}

// Remove removes the file or empty directory name; a symlink is removed, never followed.
func (r *Root) Remove(name string) error {
	return check(name, r.root.Remove(name))
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
	if err != nil {
		return fmt.Errorf("%s -> %s: %w", oldName, newName, stripPath(err))
	}
	return nil
}

// ReadDir lists the directory name (sorted by name; entries describe
// symlinks as symlinks).
func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	return readDir(r.root, name)
}

func readDir(root *os.Root, name string) ([]fs.DirEntry, error) {
	return readDirBudget(root, name, nil)
}

// ErrTooManyEntries: a bounded walk (WalkDirBounded) spent its entry budget.
var ErrTooManyEntries = errors.New("more entries than the walk's budget")

// readDirBatch is how many entries a bounded walk reads at a time.
const readDirBatch = 256

// readDirBudget is readDir that, with a budget, reads the directory in
// batches and takes each entry from *budget: once it is spent, it stops
// with ErrTooManyEntries before the rest of the directory is read.
func readDirBudget(root *os.Root, name string, budget *int) ([]fs.DirEntry, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|openNonBlock|openDirectory, 0)
	if err != nil {
		return nil, check(name, err)
	}
	defer func() { _ = f.Close() }()
	var entries []fs.DirEntry
	if budget == nil {
		entries, err = f.ReadDir(-1)
	} else {
		entries, err = readDirBatches(f, budget)
		if errors.Is(err, ErrTooManyEntries) {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	if err != nil {
		return entries, check(name, err)
	}
	return entries, nil
}

func readDirBatches(f *os.File, budget *int) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	for {
		batch, err := f.ReadDir(readDirBatch)
		*budget -= len(batch)
		if *budget < 0 {
			return nil, ErrTooManyEntries
		}
		entries = append(entries, batch...)
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return entries, err
		}
	}
}

// WalkDir walks the tree below name like fs.WalkDir (same callback
// semantics; paths relative to the workspace) and never descends into a
// symlink. Each directory is read relative to its parent's descriptor, so a
// directory costs a constant number of opens; at most maxHeldDirs
// descriptors are held, deeper directories are opened from the workspace
// root one at a time.
func (r *Root) WalkDir(name string, fn fs.WalkDirFunc) error {
	return r.walk(name, nil, fn)
}

// WalkDirBounded is WalkDir that reads at most maxEntries directory entries
// in all: directories are read in batches, and once the budget is spent the
// walk ends with an error wrapping ErrTooManyEntries, without holding the
// rest of an oversized directory in memory (KI-152 review).
func (r *Root) WalkDirBounded(name string, maxEntries int, fn fs.WalkDirFunc) error {
	budget := maxEntries
	return r.walk(name, &budget, fn)
}

// walk is WalkDir with an optional entry budget (nil: unbounded).
func (r *Root) walk(name string, budget *int, fn fs.WalkDirFunc) error {
	info, err := r.Stat(name)
	if err != nil {
		err = fn(name, nil, err)
	} else {
		var dir *os.Root
		if info.IsDir() {
			if dir, err = r.root.OpenRoot(name + string(filepath.Separator) + "."); err != nil {
				dir = nil // read through the workspace root instead
			}
		}
		err = r.walkDir(name, fs.FileInfoToDirEntry(info), dir, 1, budget, fn)
	}
	if errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

// walkDir is fs.WalkDir's walkDir; dir, when not nil, is the open directory
// name (closed here); budget, when not nil, bounds the entries read.
func (r *Root) walkDir(name string, d fs.DirEntry, dir *os.Root, held int, budget *int, fn fs.WalkDirFunc) error {
	if dir != nil {
		defer func() { _ = dir.Close() }()
	}
	if err := fn(name, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, fs.SkipDir) && d.IsDir() {
			err = nil
		}
		return err
	}
	var entries []fs.DirEntry
	var err error
	if dir != nil {
		entries, err = readDirBudget(dir, ".", budget)
	} else {
		entries, err = readDirBudget(r.root, name, budget)
	}
	if errors.Is(err, ErrTooManyEntries) {
		return err // ends the walk, whatever fn would do with the error
	}
	if err != nil {
		// Second call, to report the ReadDir error.
		if err = fn(name, d, err); err != nil {
			if errors.Is(err, fs.SkipDir) && d.IsDir() {
				err = nil
			}
			return err
		}
	}
	for _, entry := range entries {
		childName := path.Join(name, entry.Name())
		var child *os.Root
		if entry.IsDir() {
			child = r.openChild(dir, name, entry.Name(), held < maxHeldDirs)
		}
		if err := r.walkDir(childName, entry, child, held+1, budget, fn); err != nil {
			if errors.Is(err, fs.SkipDir) {
				break
			}
			return err
		}
	}
	return nil
}

// openChild opens the subdirectory child of the open directory parent (nil:
// read it through the workspace root later). A child swapped for a symlink
// since it was listed is not opened through the link: the opened directory
// must be the one the parent lists under that name.
func (r *Root) openChild(parent *os.Root, parentName, child string, hold bool) *os.Root {
	if parent == nil || !hold {
		return nil
	}
	sub, err := parent.OpenRoot(child + string(filepath.Separator) + ".")
	if err != nil {
		return nil
	}
	opened, err := sub.Stat(".")
	listed, lerr := parent.Lstat(child)
	if err != nil || lerr != nil || listed.Mode()&fs.ModeSymlink != 0 || !os.SameFile(opened, listed) {
		_ = sub.Close()
		slog.Debug("workspace walk: directory swapped, not descended", "dir", path.Join(parentName, child))
		return nil
	}
	return sub
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
	file, err := f.r.root.OpenFile(name, os.O_RDONLY|openNonBlock, 0)
	if err != nil {
		return nil, check(name, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, check(name, err)
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
