// Package proctemp gives the Go Core process one private temporary
// directory for its short-lived files (per-run checkpoint indexes, the svn
// client configuration, scratch index files) and removes the directories
// of earlier processes at startup (S3 follow-up 1e).
//
// The directory is <TMPDIR>/codeforge-core-<pid>-<start>, where start is the
// process start time from /proc: a restarted process that got the same PID
// has another name. RemoveStale removes only directories with that name
// pattern, owned by the current user, that are real directories (no
// symlinks) and whose process no longer runs. Without /proc nothing is
// considered stale.
package proctemp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const prefix = "codeforge-core-"

var namePattern = regexp.MustCompile(`^codeforge-core-(\d+)-(\d+)$`)

var (
	mu   sync.Mutex
	dirs = map[string]string{} // base -> this process's directory
)

// Dir returns this process's temporary directory in os.TempDir(), creating
// it (0700) on first use.
func Dir() (string, error) {
	return dirFor(os.TempDir())
}

// MkdirTemp creates a new directory in this process's temporary directory,
// like os.MkdirTemp.
func MkdirTemp(pattern string) (string, error) {
	parent, err := Dir()
	if err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, pattern)
}

func dirFor(base string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if dir, ok := dirs[base]; ok {
		return dir, nil
	}
	start, ok := processStart(os.Getpid())
	if !ok {
		start = "0"
	}
	dir := filepath.Join(base, fmt.Sprintf("%s%d-%s", prefix, os.Getpid(), start))
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("process temp dir: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("process temp dir: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ownedByUs(info) {
		return "", fmt.Errorf("process temp dir %s is not a private directory of this user", dir)
	}
	dirs[base] = dir
	return dir, nil
}

// RemoveStale removes, in base, the temporary directories of Go Core
// processes that no longer run; it returns how many it removed.
func RemoveStale(base string) (int, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0, err
	}
	removed := 0
	var errs []error
	for _, e := range entries {
		m := namePattern.FindStringSubmatch(e.Name())
		if m == nil || !e.IsDir() { // e.IsDir is false for a symlink
			continue
		}
		path := filepath.Join(base, e.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || !ownedByUs(info) {
			continue
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil || running(pid, m[2]) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// running reports whether the process pid that started at start still runs.
// Without /proc it cannot tell and says yes.
func running(pid int, start string) bool {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		return true
	}
	current, ok := processStart(pid)
	return ok && current == start
}

// processStart returns the start time of process pid (field 22 of
// /proc/<pid>/stat, clock ticks since boot).
func processStart(pid int) (string, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)) //nolint:gosec // procfs path built from a number
	if err != nil {
		return "", false
	}
	// The command name (field 2) may contain spaces; fields after it follow
	// the last ')'.
	i := strings.LastIndexByte(string(data), ')')
	if i < 0 {
		return "", false
	}
	fields := strings.Fields(string(data)[i+1:])
	const startField = 22 - 3 // fields after ')' begin with field 3
	if len(fields) <= startField {
		return "", false
	}
	return fields[startField], true
}
