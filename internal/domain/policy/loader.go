package policy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads a single PolicyProfile from a YAML file.
func LoadFromFile(path string) (*PolicyProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file %s: %w", path, err)
	}

	var p PolicyProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse policy file %s: %w", path, err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate policy file %s: %w", path, err)
	}

	return &p, nil
}

// LoadFromDirectory reads all .yaml/.yml files from a directory
// and returns a slice of PolicyProfiles. Missing directories return
// an empty slice (not an error), matching the existing config pattern.
func LoadFromDirectory(dir string) ([]PolicyProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read policy directory %s: %w", dir, err)
	}

	var profiles []PolicyProfile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		p, err := LoadFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}

	return profiles, nil
}
