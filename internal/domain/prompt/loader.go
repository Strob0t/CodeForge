package prompt

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile parses a single YAML document into a PromptEntry and validates it.
func LoadFile(data []byte) (PromptEntry, error) {
	var entry PromptEntry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return PromptEntry{}, fmt.Errorf("yaml parse error: %w", err)
	}
	if err := validateEntry(&entry); err != nil {
		return PromptEntry{}, err
	}
	return entry, nil
}

// LoadFS walks a directory tree via the given fs.FS, loads all .yaml files,
// and returns the aggregated prompt entries. If any file fails validation,
// an error referencing the file path is returned.
func LoadFS(fsys fs.FS, root string) ([]PromptEntry, error) {
	var entries []PromptEntry

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk error at %s: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}

		entry, loadErr := LoadFile(data)
		if loadErr != nil {
			return fmt.Errorf("load %s: %w", path, loadErr)
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// validateEntry checks that all required fields are present and values are in range.
func validateEntry(e *PromptEntry) error {
	if e.ID == "" {
		return fmt.Errorf("validation: missing required field 'id'")
	}
	if e.Category == "" {
		return fmt.Errorf("validation: missing required field 'category'")
	}
	if e.Name == "" {
		return fmt.Errorf("validation: missing required field 'name'")
	}
	if e.Content == "" {
		return fmt.Errorf("validation: missing required field 'content'")
	}
	if !ValidCategory(e.Category) {
		return fmt.Errorf("validation: invalid category %q", e.Category)
	}
	if e.Priority < 0 || e.Priority > 100 {
		return fmt.Errorf("validation: priority must be 0-100, got %d", e.Priority)
	}
	return nil
}
