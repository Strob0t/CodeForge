package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/filesystem"
)

// FileEntry represents a file or directory in a project workspace.
type FileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// FileContent represents the content and metadata of a file.
type FileContent struct {
	Path     string    `json:"path"`
	Content  string    `json:"content"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Language string    `json:"language"`
}

// FileService provides file operations scoped to project workspaces.
type FileService struct {
	store database.Store
	fs    filesystem.Provider
}

// NewFileService creates a new FileService.
func NewFileService(store database.Store, fs filesystem.Provider) *FileService {
	return &FileService{store: store, fs: fs}
}

// ListDirectory lists files and directories at the given path within a project workspace.
func (s *FileService) ListDirectory(ctx context.Context, projectID, relPath string) ([]FileEntry, error) {
	absPath, err := s.resolveProjectPath(ctx, projectID, relPath)
	if err != nil {
		return nil, err
	}

	info, err := s.fs.Stat(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", relPath)
	}

	entries, err := s.fs.ReadDir(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		fi, fiErr := entry.Info()
		if fiErr != nil {
			continue
		}
		entryPath := filepath.Join(relPath, entry.Name())
		// Normalize to forward slashes for consistent API output
		entryPath = filepath.ToSlash(entryPath)

		result = append(result, FileEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsDir:   entry.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}

	return result, nil
}

// ListTree recursively lists all files and directories within a project workspace.
func (s *FileService) ListTree(ctx context.Context, projectID string, maxEntries int) ([]FileEntry, error) {
	absPath, err := s.resolveProjectPath(ctx, projectID, ".")
	if err != nil {
		return nil, err
	}

	result := make([]FileEntry, 0, 256)
	err = s.fs.WalkDir(ctx, absPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if len(result) >= maxEntries {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(absPath, path)
		if relErr != nil || rel == "." {
			return nil
		}
		// Skip .git directory (large, irrelevant for file browsing)
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		fi, fiErr := d.Info()
		if fiErr != nil {
			return nil
		}
		result = append(result, FileEntry{
			Name:    d.Name(),
			Path:    filepath.ToSlash(rel),
			IsDir:   d.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return result, nil
}

// ReadFile reads the content of a file within a project workspace.
func (s *FileService) ReadFile(ctx context.Context, projectID, relPath string) (*FileContent, error) {
	absPath, err := s.resolveProjectPath(ctx, projectID, relPath)
	if err != nil {
		return nil, err
	}

	info, err := s.fs.Stat(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("file does not exist: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", relPath)
	}

	// Limit file size to 10 MB to prevent OOM
	const maxFileSize = 10 * 1024 * 1024
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}

	data, err := s.fs.ReadFile(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return &FileContent{
		Path:     filepath.ToSlash(relPath),
		Content:  string(data),
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Language: detectLanguage(relPath),
	}, nil
}

// WriteFile writes content to a file within a project workspace.
func (s *FileService) WriteFile(ctx context.Context, projectID, relPath, content string) error {
	absPath, err := s.resolveProjectPath(ctx, projectID, relPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	if err := s.fs.MkdirAll(ctx, dir, 0o750); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	if err := s.fs.WriteFile(ctx, absPath, []byte(content), fs.FileMode(0o644)); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// DeleteFile removes a file or directory within a project workspace.
func (s *FileService) DeleteFile(ctx context.Context, projectID, relPath string) error {
	absPath, err := s.resolveProjectPath(ctx, projectID, relPath)
	if err != nil {
		return err
	}

	if _, statErr := s.fs.Stat(ctx, absPath); statErr != nil {
		return fmt.Errorf("path does not exist: %w", statErr)
	}

	if err := s.fs.RemoveAll(ctx, absPath); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	return nil
}

// RenameFile moves/renames a file or directory within a project workspace.
func (s *FileService) RenameFile(ctx context.Context, projectID, oldRelPath, newRelPath string) error {
	oldAbs, err := s.resolveProjectPath(ctx, projectID, oldRelPath)
	if err != nil {
		return fmt.Errorf("resolve old path: %w", err)
	}
	newAbs, err := s.resolveProjectPath(ctx, projectID, newRelPath)
	if err != nil {
		return fmt.Errorf("resolve new path: %w", err)
	}

	if _, statErr := s.fs.Stat(ctx, oldAbs); statErr != nil {
		return fmt.Errorf("source does not exist: %w", statErr)
	}

	// Ensure parent directory of destination exists
	if mkErr := s.fs.MkdirAll(ctx, filepath.Dir(newAbs), 0o750); mkErr != nil {
		return fmt.Errorf("create parent directory: %w", mkErr)
	}

	if err := s.fs.Rename(ctx, oldAbs, newAbs); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

// resolveProjectPath resolves a relative path to an absolute path within a project workspace,
// with path traversal protection.
func (s *FileService) resolveProjectPath(ctx context.Context, projectID, relPath string) (string, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return "", fmt.Errorf("project %s has no workspace", projectID)
	}

	wsPath, err := filepath.Abs(p.WorkspacePath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}

	// Clean and join the relative path
	cleaned := filepath.Clean(relPath)
	absPath := filepath.Join(wsPath, cleaned)

	// Resolve symlinks to prevent symlink-based traversal
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If file doesn't exist yet (write case), verify parent directory
		if os.IsNotExist(err) {
			parentResolved, parentErr := filepath.EvalSymlinks(filepath.Dir(absPath))
			if parentErr != nil {
				return "", fmt.Errorf("resolve parent path: %w", parentErr)
			}
			if !strings.HasPrefix(parentResolved+string(filepath.Separator), wsPath+string(filepath.Separator)) &&
				parentResolved != wsPath {
				return "", fmt.Errorf("path traversal denied: %s", relPath)
			}
			return absPath, nil
		}
		return "", fmt.Errorf("resolve path: %w", err)
	}

	// Verify resolved path is within workspace
	if !strings.HasPrefix(resolved+string(filepath.Separator), wsPath+string(filepath.Separator)) &&
		resolved != wsPath {
		return "", fmt.Errorf("path traversal denied: %s", relPath)
	}

	return resolved, nil
}

// detectLanguage returns a language identifier based on file extension.
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	languages := map[string]string{
		".go":         "go",
		".py":         "python",
		".js":         "javascript",
		".jsx":        "javascript",
		".ts":         "typescript",
		".tsx":        "typescript",
		".html":       "html",
		".css":        "css",
		".scss":       "scss",
		".json":       "json",
		".yaml":       "yaml",
		".yml":        "yaml",
		".xml":        "xml",
		".md":         "markdown",
		".sql":        "sql",
		".sh":         "shell",
		".bash":       "shell",
		".rs":         "rust",
		".java":       "java",
		".c":          "c",
		".cpp":        "cpp",
		".h":          "c",
		".hpp":        "cpp",
		".rb":         "ruby",
		".php":        "php",
		".swift":      "swift",
		".kt":         "kotlin",
		".toml":       "toml",
		".ini":        "ini",
		".r":          "r",
		".lua":        "lua",
		".vim":        "vim",
		".proto":      "protobuf",
		".graphql":    "graphql",
		".svg":        "xml",
		".dockerfile": "dockerfile",
	}
	if lang, ok := languages[ext]; ok {
		return lang
	}
	// Check filename-based detection
	base := strings.ToLower(filepath.Base(path))
	filenames := map[string]string{
		"dockerfile":     "dockerfile",
		"makefile":       "makefile",
		"cmakelists.txt": "cmake",
		".gitignore":     "gitignore",
		".env":           "dotenv",
	}
	if lang, ok := filenames[base]; ok {
		return lang
	}
	return "plaintext"
}
