package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads a single Template from a YAML file.
func LoadFromFile(path string) (*Template, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("read pipeline file %s: %w", path, err)
	}

	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse pipeline file %s: %w", path, err)
	}

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validate pipeline file %s: %w", path, err)
	}

	return &t, nil
}

// LoadFromDirectory reads all .yaml/.yml files from a directory
// and returns a slice of Templates. Missing directories return
// an empty slice (not an error), matching the policy loader pattern.
func LoadFromDirectory(dir string) ([]Template, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pipeline directory %s: %w", dir, err)
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

		t, err := LoadFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		templates = append(templates, *t)
	}

	return templates, nil
}
