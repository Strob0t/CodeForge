// Package filesystem defines the port interface for filesystem operations.
package filesystem

import (
	"context"
	"io"
	"io/fs"
)

// Provider abstracts filesystem operations for service layer decoupling.
type Provider interface {
	Stat(ctx context.Context, path string) (fs.FileInfo, error)
	ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
	MkdirAll(ctx context.Context, path string, perm fs.FileMode) error
	Remove(ctx context.Context, path string) error
	RemoveAll(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Create(ctx context.Context, path string) (io.WriteCloser, error)
	WalkDir(ctx context.Context, root string, fn fs.WalkDirFunc) error
}
