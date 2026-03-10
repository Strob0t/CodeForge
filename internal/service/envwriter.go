package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvWriter reads and writes key=value pairs in a .env file atomically.
type EnvWriter struct {
	path string
}

// NewEnvWriter creates an EnvWriter for the given .env file path.
func NewEnvWriter(path string) *EnvWriter {
	return &EnvWriter{path: path}
}

// Get returns the value for key, or empty string if not found.
func (w *EnvWriter) Get(key string) (string, error) {
	entries, err := w.readAll()
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.key == key {
			return e.value, nil
		}
	}
	return "", nil
}

// Set writes or updates a key=value pair in the .env file.
// The write is atomic: data is written to a temp file, then renamed.
func (w *EnvWriter) Set(key, value string) error {
	entries, err := w.readAll()
	if os.IsNotExist(err) {
		entries = nil
	} else if err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	found := false
	for i := range entries {
		if entries[i].key == key {
			entries[i].value = value
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, envEntry{key: key, value: value})
	}

	return w.writeAll(entries)
}

// Delete removes a key from the .env file. No error if key doesn't exist.
func (w *EnvWriter) Delete(key string) error {
	entries, err := w.readAll()
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	filtered := make([]envEntry, 0, len(entries))
	for _, e := range entries {
		if e.key != key {
			filtered = append(filtered, e)
		}
	}

	return w.writeAll(filtered)
}

// Has returns true if the key exists and has a non-empty value.
func (w *EnvWriter) Has(key string) (bool, error) {
	val, err := w.Get(key)
	if err != nil {
		return false, err
	}
	return val != "", nil
}

// envEntry represents a line in the .env file.
// Comments and blank lines are preserved as raw lines.
type envEntry struct {
	key   string // empty for comments/blanks
	value string
	raw   string // original line for comments/blanks
}

// readAll parses the .env file, preserving comments and blank lines.
func (w *EnvWriter) readAll() ([]envEntry, error) {
	f, err := os.Open(w.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []envEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			entries = append(entries, envEntry{raw: line})
			continue
		}

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			entries = append(entries, envEntry{raw: line})
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		entries = append(entries, envEntry{key: key, value: value})
	}

	return entries, scanner.Err()
}

// writeAll writes entries to the .env file atomically.
func (w *EnvWriter) writeAll(entries []envEntry) error {
	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	writer := bufio.NewWriter(tmp)
	for _, e := range entries {
		if e.key == "" {
			_, _ = writer.WriteString(e.raw)
		} else {
			_, _ = writer.WriteString(e.key + "=" + e.value)
		}
		_ = writer.WriteByte('\n')
	}
	if err := writer.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:gosec // G703: tmpName from os.CreateTemp, not user input
		return fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // G703: tmpName from os.CreateTemp, not user input
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, w.path); err != nil { //nolint:gosec // G703: tmpName from os.CreateTemp, not user input
		_ = os.Remove(tmpName) //nolint:gosec // G703: tmpName from os.CreateTemp, not user input
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
