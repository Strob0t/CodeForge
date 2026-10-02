package service

import (
	"errors"
	"fmt"
	"log/slog"
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
// stored relative to the root and resolved inside it (operatorDir).
type knowledgeContent struct {
	dir    operatorDir
	logged sync.Map // IDs of knowledge bases whose refused content_path was logged
}

func newKnowledgeContent(root string) *knowledgeContent {
	return &knowledgeContent{dir: newOperatorDir(root)}
}

// relative returns contentPath as a clean slash path relative to the root:
// a relative path that stays below it, or an absolute path that lies inside
// it (lexically, as configured or resolved).
func (k *knowledgeContent) relative(contentPath string) (string, error) {
	rel, ok := k.dir.relative(contentPath)
	if !ok {
		return "", fmt.Errorf("%w: %w", domain.ErrValidation, errOutsideContentRoot)
	}
	return rel, nil
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
	return k.dir.open()
}
