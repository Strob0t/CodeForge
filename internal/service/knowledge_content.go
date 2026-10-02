package service

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// errOutsideContentRoot refuses a knowledge-base content_path outside the
// knowledge content root.
var errOutsideContentRoot = errors.New("content_path must lie inside the knowledge content root (knowledge.content_root)")

// knowledgeContent places knowledge-base content below the operator's
// content root (knowledge.content_root, KI-105). Before, content_path was
// any absolute path: the Go Core read it into agent context and the worker
// indexed it, so any user could read files of the Core container. Paths are
// stored relative to the root and resolved inside it through workspacefs
// (os.Root): no symlink leads out of it, a FIFO never blocks, only regular
// files are read.
type knowledgeContent struct {
	root   string   // the content root, absolute as configured
	roots  []string // root and its resolved form: an absolute input may name either
	logged sync.Map // IDs of knowledge bases whose refused content_path was logged
}

func newKnowledgeContent(root string) *knowledgeContent {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = filepath.Clean(root)
	}
	k := &knowledgeContent{root: abs, roots: []string{abs}}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		k.roots = append(k.roots, resolved)
	}
	return k
}

// relative returns contentPath as a clean slash path relative to the root:
// a relative path that stays below it, or an absolute path that lies inside
// it (lexically, as configured or resolved).
func (k *knowledgeContent) relative(contentPath string) (string, error) {
	if filepath.IsAbs(contentPath) {
		clean := filepath.Clean(contentPath)
		for _, root := range k.roots {
			if rel, err := filepath.Rel(root, clean); err == nil && filepath.IsLocal(rel) {
				return filepath.ToSlash(rel), nil
			}
		}
		return "", fmt.Errorf("%w: %w", domain.ErrValidation, errOutsideContentRoot)
	}
	if !filepath.IsLocal(contentPath) {
		return "", fmt.Errorf("%w: %w", domain.ErrValidation, errOutsideContentRoot)
	}
	return filepath.ToSlash(filepath.Clean(contentPath)), nil
}

// stored returns the relative content path of kb. A row written before the
// content root (an absolute path outside it) is refused; that is logged once
// per knowledge base.
func (k *knowledgeContent) stored(kb *knowledgebase.KnowledgeBase) (string, error) {
	rel, err := k.relative(kb.ContentPath)
	if err != nil {
		if _, seen := k.logged.LoadOrStore(kb.ID, true); !seen {
			slog.Warn("knowledge base content_path outside the content root, refused (KI-105)",
				"kb_id", kb.ID, "content_path", kb.ContentPath)
		}
		return "", err
	}
	return rel, nil
}

// open opens the content root.
func (k *knowledgeContent) open() (*workspacefs.Root, error) {
	return workspacefs.OpenOperatorDir(k.root)
}
