package policy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileWriter abstracts file write operations so the domain layer
// does not depend on the os package for persistence.
type FileWriter interface {
	WriteFile(path string, data []byte, perm fs.FileMode) error
}

// osFileWriter is the production implementation backed by os.WriteFile.
type osFileWriter struct{}

func (osFileWriter) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// OSFileWriter returns a FileWriter that writes to the real filesystem.
func OSFileWriter() FileWriter { return osFileWriter{} }

// LoadFromFS reads a single PolicyProfile from a YAML file within the given filesystem.
func LoadFromFS(fsys fs.FS, name string) (*PolicyProfile, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read policy file %s: %w", name, err)
	}

	var p PolicyProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse policy file %s: %w", name, err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate policy file %s: %w", name, err)
	}

	return &p, nil
}

// LoadAllFromFS reads all .yaml/.yml files from the given filesystem
// and returns a slice of PolicyProfiles.
func LoadAllFromFS(fsys fs.FS) ([]PolicyProfile, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read policy directory: %w", err)
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

		p, err := LoadFromFS(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}

	return profiles, nil
}

// LoadFromFile reads a single PolicyProfile from a YAML file on disk.
// It delegates to LoadFromFS using os.DirFS.
func LoadFromFile(path string) (*PolicyProfile, error) {
	return LoadFromFS(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// LoadFromDirectory reads all .yaml/.yml files from a directory on disk
// and returns a slice of PolicyProfiles. Missing directories return
// an empty slice (not an error), matching the existing config pattern.
func LoadFromDirectory(dir string) ([]PolicyProfile, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read policy directory %s: %w", dir, err)
	}
	return LoadAllFromFS(os.DirFS(dir))
}

// SaveToFile writes a PolicyProfile to a YAML file using the provided writer.
func SaveToFile(path string, profile *PolicyProfile) error {
	return SaveToFileWith(OSFileWriter(), path, profile)
}

// SaveToFileWith writes a PolicyProfile to a YAML file using the given FileWriter.
func SaveToFileWith(w FileWriter, path string, profile *PolicyProfile) error {
	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal policy profile: %w", err)
	}
	if err := w.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write policy file %s: %w", path, err)
	}
	return nil
}
