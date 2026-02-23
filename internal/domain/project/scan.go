package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxManifestRead is the maximum bytes to read from a manifest file for framework detection.
const maxManifestRead = 64 * 1024

// ScanWorkspace scans a directory for language manifests and returns detection results.
// Only top-level entries are checked (no recursive walk).
func ScanWorkspace(path string) (*StackDetectionResult, error) {
	info, err := os.Stat(path) //nolint:gosec // path comes from trusted workspace config or explicit user request
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan workspace: %s is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: read dir: %w", err)
	}

	// Collect which manifests exist per language.
	langManifests := make(map[string][]string)  // language → list of manifest filenames
	manifestContents := make(map[string]string) // filename → file content (cached for framework detection)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lang, ok := manifestMap[entry.Name()]
		if !ok {
			continue
		}
		langManifests[lang] = append(langManifests[lang], entry.Name())

		// Read content for framework detection (lazy, capped at maxManifestRead).
		if _, cached := manifestContents[entry.Name()]; !cached {
			content := readFileCapped(filepath.Join(path, entry.Name()), maxManifestRead)
			manifestContents[entry.Name()] = content
		}
	}

	// If tsconfig.json was found but package.json was also found, promote to typescript.
	// tsconfig.json alone indicates TypeScript; package.json alone indicates JavaScript.
	if _, hasTS := langManifests["typescript"]; hasTS {
		if jsManifests, hasJS := langManifests["javascript"]; hasJS {
			// Merge JS manifests into TS and remove JS entry.
			langManifests["typescript"] = append(langManifests["typescript"], jsManifests...)
			delete(langManifests, "javascript")
		}
	}

	// Build Language entries.
	languages := make([]Language, 0, len(langManifests))
	for lang, manifests := range langManifests {
		confidence := manifestConfidence(len(manifests))
		frameworks := detectFrameworks(lang, manifestContents)

		languages = append(languages, Language{
			Name:       lang,
			Confidence: confidence,
			Manifests:  manifests,
			Frameworks: frameworks,
		})
	}

	// Deduplicate recommendations across languages.
	seen := make(map[string]bool) // category+id
	var recs []ToolRecommendation
	for _, lang := range languages {
		for _, rec := range RecommendationsForLanguage(lang.Name) {
			key := rec.Category + ":" + rec.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			recs = append(recs, rec)
		}
	}

	return &StackDetectionResult{
		Languages:       languages,
		Recommendations: recs,
		ScannedPath:     path,
	}, nil
}

// manifestConfidence returns a confidence score based on the number of manifests found.
func manifestConfidence(count int) float64 {
	switch {
	case count >= 3:
		return 1.0
	case count == 2:
		return 0.9
	default:
		return 0.7
	}
}

// detectFrameworks checks manifest contents for known framework signatures.
func detectFrameworks(lang string, contents map[string]string) []string {
	rules, ok := frameworkDetectors[lang]
	if !ok {
		return nil
	}

	seen := make(map[string]bool)
	var frameworks []string
	for _, rule := range rules {
		content, ok := contents[rule.Manifest]
		if !ok || content == "" {
			continue
		}
		if strings.Contains(content, rule.Substring) && !seen[rule.Framework] {
			seen[rule.Framework] = true
			frameworks = append(frameworks, rule.Framework)
		}
	}
	return frameworks
}

// readFileCapped reads up to maxBytes from a file, returning the content as a string.
// Returns empty string on any error.
func readFileCapped(path string, maxBytes int) string {
	f, err := os.Open(path) //nolint:gosec // path is constructed from workspace dir + known manifest filenames
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	return string(buf[:n])
}
