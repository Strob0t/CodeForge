// Package osfs implements the filesystem port using the OS standard library.
package osfs

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Provider implements filesystem.Provider backed by the real OS filesystem.
type Provider struct{}

// New creates a new OS-backed filesystem provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (p *Provider) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (p *Provider) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // G304: path validated by caller (service layer)
}

func (p *Provider) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (p *Provider) MkdirAll(_ context.Context, path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (p *Provider) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

func (p *Provider) RemoveAll(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

func (p *Provider) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (p *Provider) Open(_ context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path) //nolint:gosec // G304: path validated by caller (service layer)
}

func (p *Provider) Create(_ context.Context, path string) (io.WriteCloser, error) {
	return os.Create(path) //nolint:gosec // G304: path validated by caller (service layer)
}

func (p *Provider) WalkDir(_ context.Context, root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}
