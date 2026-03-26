package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFS reads a single Template from a YAML file within the given filesystem.
func LoadFromFS(fsys fs.FS, name string) (*Template, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file %s: %w", name, err)
	}

	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse pipeline file %s: %w", name, err)
	}

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validate pipeline file %s: %w", name, err)
	}

	return &t, nil
}

// LoadAllFromFS reads all .yaml/.yml files from the given filesystem
// and returns a slice of Templates.
func LoadAllFromFS(fsys fs.FS) ([]Template, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read pipeline directory: %w", err)
	}

	var templates []Template
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		t, err := LoadFromFS(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		templates = append(templates, *t)
	}

	return templates, nil
}

// LoadFromFile reads a single Template from a YAML file on disk.
// It delegates to LoadFromFS using os.DirFS.
func LoadFromFile(path string) (*Template, error) {
	return LoadFromFS(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// LoadFromDirectory reads all .yaml/.yml files from a directory on disk
// and returns a slice of Templates. Missing directories return
// an empty slice (not an error), matching the policy loader pattern.
func LoadFromDirectory(dir string) ([]Template, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pipeline directory %s: %w", dir, err)
	}
	return LoadAllFromFS(os.DirFS(dir))
}
