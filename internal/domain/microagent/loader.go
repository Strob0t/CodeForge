package microagent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFromFile loads a microagent from a YAML front matter + Markdown body file.
// Format:
//
//	name: python-testing
//	type: knowledge
//	trigger: "test_*.py"
//	---
//	When working with Python test files, always use pytest fixtures...
func LoadFromFile(path string) (*Microagent, error) {
	//nolint:gosec // G304: path comes from trusted config directory
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open microagent file %s: %w", path, err)
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
		return nil, fmt.Errorf("read microagent file %s: %w", path, err)
	}

	m.Prompt = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid microagent %s: %w", path, err)
	}
	return m, nil
}

// LoadFromDirectory loads all microagent files (*.md) from a directory.
func LoadFromDirectory(dir string) ([]*Microagent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read microagent directory %s: %w", dir, err)
	}

	var agents []*Microagent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		m, err := LoadFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		agents = append(agents, m)
	}
	return agents, nil
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
