package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
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

// FileService provides file operations scoped to project workspaces. Every
// path is resolved inside the project's workspace by workspacefs (os.Root,
// KI-95): a symlink never leads out of it, a FIFO never blocks a request,
// and symlinks are deleted and renamed themselves, never their targets.
type FileService struct {
	store database.Store
}

// NewFileService creates a new FileService.
func NewFileService(store database.Store) *FileService {
	return &FileService{store: store}
}

// maxFileSize caps the files ReadFile returns, to prevent OOM.
const maxFileSize = 10 * 1024 * 1024

// ListDirectory lists files and directories at the given path within a project workspace.
func (s *FileService) ListDirectory(ctx context.Context, projectID, relPath string) ([]FileEntry, error) {
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ws.Close() }()
	name := workspaceName(relPath)

	info, err := ws.Stat(name)
	if err != nil {
		return nil, refused(err, "path does not exist")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", relPath)
	}

	entries, err := ws.ReadDir(name)
	if err != nil {
		return nil, refused(err, "read directory")
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

// ListTree recursively lists all files and directories within a project
// workspace; symlinks are listed, never descended.
func (s *FileService) ListTree(ctx context.Context, projectID string, maxEntries int) ([]FileEntry, error) {
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ws.Close() }()

	result := make([]FileEntry, 0, 256)
	err = ws.WalkDir(".", func(entryPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if len(result) >= maxEntries {
			return filepath.SkipAll
		}
		if entryPath == "." {
			return nil
		}
		// Skip the git directory (large, irrelevant for file browsing)
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		fi, fiErr := d.Info()
		if fiErr != nil {
			return nil
		}
		result = append(result, FileEntry{
			Name:    d.Name(),
			Path:    entryPath,
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
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ws.Close() }()
	name := workspaceName(relPath)

	info, err := ws.Stat(name)
	if err != nil {
		return nil, refused(err, "file does not exist")
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", relPath)
	}

	data, info, err := ws.ReadFile(name, maxFileSize)
	if err != nil {
		return nil, refused(err, "read file")
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
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	name := workspaceName(relPath)

	// Ensure parent directory exists
	if err := ws.MkdirAll(filepath.Dir(name), project.WorkspaceDirPerm); err != nil {
		return refused(err, "create parent directory")
	}

	if err := ws.WriteFile(name, []byte(content), project.WorkspaceFilePerm); err != nil {
		return refused(err, "write file")
	}

	return nil
}

// DeleteFile removes a file or directory within a project workspace (a
// symlink itself, never its target).
func (s *FileService) DeleteFile(ctx context.Context, projectID, relPath string) error {
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	name := workspaceName(relPath)
	if name == "." {
		return errWorkspaceRoot
	}

	if _, statErr := ws.Lstat(name); statErr != nil {
		return refused(statErr, "path does not exist")
	}

	if err := ws.RemoveAll(name); err != nil {
		return refused(err, "delete failed")
	}
	return nil
}

// RenameFile moves/renames a file or directory within a project workspace
// (a symlink itself, never its target).
func (s *FileService) RenameFile(ctx context.Context, projectID, oldRelPath, newRelPath string) error {
	ws, err := s.openWorkspace(ctx, projectID)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	oldName, newName := workspaceName(oldRelPath), workspaceName(newRelPath)
	if oldName == "." || newName == "." {
		return errWorkspaceRoot
	}

	if _, statErr := ws.Lstat(oldName); statErr != nil {
		return refused(statErr, "source does not exist")
	}

	// Ensure parent directory of destination exists
	if mkErr := ws.MkdirAll(filepath.Dir(newName), project.WorkspaceDirPerm); mkErr != nil {
		return refused(mkErr, "create parent directory")
	}

	if err := ws.Rename(oldName, newName); err != nil {
		return refused(err, "rename failed")
	}
	return nil
}

// openWorkspace opens the project's workspace directory.
func (s *FileService) openWorkspace(ctx context.Context, projectID string) (*workspacefs.Root, error) {
	p, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return nil, fmt.Errorf("project %s has no workspace", projectID)
	}
	ws, err := workspacefs.Open(p.WorkspacePath)
	if err != nil {
		return nil, refused(err, "open workspace")
	}
	return ws, nil
}

// errWorkspaceRoot refuses deleting or renaming the workspace itself.
var errWorkspaceRoot = fmt.Errorf("%w: the workspace root cannot be deleted or renamed", domain.ErrValidation)

// workspaceName turns an API path into a cleaned name inside the workspace
// ("." for the workspace itself). API paths are workspace-relative; a
// leading slash was always ignored. Cleaning comes first, so a name like
// "x/.." is recognised as the workspace root before any check; ".." that
// climbs out stays and is refused by os.Root.
func workspaceName(relPath string) string {
	return path.Clean(strings.TrimLeft(filepath.ToSlash(relPath), "/"))
}

// refused wraps err with what failed; a path workspacefs refused (it leaves
// the workspace, is not a regular file, is too large) is the client's
// error (400) with a clear message.
func refused(err error, what string) error {
	if errors.Is(err, workspacefs.ErrLeavesWorkspace) || errors.Is(err, workspacefs.ErrNotRegular) ||
		errors.Is(err, workspacefs.ErrTooLarge) {
		return fmt.Errorf("%w: %w", domain.ErrValidation, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// detectLanguage returns a language identifier based on file extension.
func detectLanguage(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
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
	base := strings.ToLower(filepath.Base(name))
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
