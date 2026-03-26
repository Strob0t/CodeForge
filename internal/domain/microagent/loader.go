package microagent

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// LoadFromFS loads a microagent from a YAML front matter + Markdown body file
// within the given filesystem.
// Format:
//
//	name: python-testing
//	type: knowledge
//	trigger: "test_*.py"
//	---
//	When working with Python test files, always use pytest fixtures...
func LoadFromFS(fsys fs.FS, name string) (*Microagent, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open microagent file %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	m := &Microagent{Enabled: true}
	scanner := bufio.NewScanner(f)
	inFrontMatter := true
	var bodyLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if inFrontMatter {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				inFrontMatter = false
				continue
			}
			key, value, ok := parseYAMLLine(trimmed)
			if !ok {
				continue
			}
			switch key {
			case "name":
				m.Name = value
			case "type":
				m.Type = Type(value)
			case "trigger":
				m.TriggerPattern = value
			case "description":
				m.Description = value
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read microagent file %s: %w", name, err)
	}

	m.Prompt = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid microagent %s: %w", name, err)
	}
	return m, nil
}

// LoadAllFromFS loads all microagent files (*.md) from the given filesystem.
func LoadAllFromFS(fsys fs.FS) ([]*Microagent, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read microagent directory: %w", err)
	}

	var agents []*Microagent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		m, err := LoadFromFS(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		agents = append(agents, m)
	}
	return agents, nil
}

// LoadFromFile loads a microagent from a YAML front matter + Markdown body file on disk.
// It delegates to LoadFromFS using os.DirFS.
func LoadFromFile(path string) (*Microagent, error) {
	dir, base := splitPath(path)
	return LoadFromFS(os.DirFS(dir), base)
}

// LoadFromDirectory loads all microagent files (*.md) from a directory on disk.
// Missing directories return an empty slice (not an error).
func LoadFromDirectory(dir string) ([]*Microagent, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read microagent directory %s: %w", dir, err)
	}
	return LoadAllFromFS(os.DirFS(dir))
}

// splitPath splits a file path into directory and base name.
// If the path has no directory component, "." is returned as the directory.
func splitPath(path string) (dir, base string) {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i], path[i+1:]
		}
	}
	return ".", path
}

// parseYAMLLine extracts a key-value pair from a simple "key: value" YAML line.
func parseYAMLLine(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	// Strip surrounding quotes
	value = strings.Trim(value, `"'`)
	return key, value, true
}
