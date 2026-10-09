package service

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// errKnowledgeUnavailable is the one answer for a content_path that lies
// outside the caller's area, is missing, leads out of the area or is no
// file or directory: no answer tells another tenant's paths apart (KI-105).
var errKnowledgeUnavailable = errors.New("content_path is not available in this tenant's knowledge area " +
	"(knowledge.content_root/<tenant>)")

// knowledgeUnavailable is errKnowledgeUnavailable as a validation error (400).
func knowledgeUnavailable() error {
	return fmt.Errorf("%w: %w", domain.ErrValidation, errKnowledgeUnavailable)
}

// knowledgeContent places knowledge-base content below the operator's
// content root (knowledge.content_root, KI-105), one area per tenant like the
// workspace root: <content_root>/<tenant_id>/. content_path is stored
// relative to the tenant's area ("." is the whole area) and resolved inside
// it through workspacefs (os.Root): no symlink leads out of the area, a FIFO
// never blocks, only regular files are read. Before, content_path was any
// absolute path the Go Core read into agent context and the worker indexed.
type knowledgeContent struct {
	dir    operatorDir
	logged sync.Map // IDs of knowledge bases whose refused content_path was logged
}

func newKnowledgeContent(root string) *knowledgeContent {
	return &knowledgeContent{dir: newOperatorDir(root)}
}

// area returns the tenant's area below the content root; false for a tenant
// that is no canonical UUID (it must be one path component) or when no root
// is configured.
func (k *knowledgeContent) area(tenant string) (operatorDir, bool) {
	if k.dir.path == "" {
		return operatorDir{}, false
	}
	if id, err := uuid.Parse(tenant); err != nil || id.String() != tenant {
		return operatorDir{}, false
	}
	area := operatorDir{path: filepath.Join(k.dir.path, tenant)}
	for _, dir := range k.dir.paths {
		area.paths = append(area.paths, filepath.Join(dir, tenant))
	}
	return area, true
}

// relative returns contentPath as a clean slash path relative to the
// tenant's area: a relative path that stays below it, or an absolute path
// that lies inside it (lexically, below the root as configured or resolved).
func (k *knowledgeContent) relative(tenant, contentPath string) (string, error) {
	area, ok := k.area(tenant)
	if !ok {
		return "", knowledgeUnavailable()
	}
	rel, ok := area.relative(contentPath)
	if !ok {
		return "", knowledgeUnavailable()
	}
	return rel, nil
}

// stored returns the relative content path of the tenant's kb. A row whose
// path does not lie in the tenant's area (an absolute path written before
// KI-105) is refused; that is logged once per knowledge base.
func (k *knowledgeContent) stored(tenant string, kb *knowledgebase.KnowledgeBase) (string, error) {
	rel, err := k.relative(tenant, kb.ContentPath)
	if err != nil {
		if _, seen := k.logged.LoadOrStore(kb.ID, true); !seen {
			slog.Warn("knowledge base content_path outside the tenant's knowledge area, refused (KI-105)",
				"kb_id", kb.ID, "tenant_id", tenant, "content_path", kb.ContentPath)
		}
		return "", err
	}
	return rel, nil
}

// open opens the tenant's area; nothing resolved through it leaves it.
func (k *knowledgeContent) open(tenant string) (*workspacefs.Root, error) {
	area, ok := k.area(tenant)
	if !ok {
		return nil, errNoOperatorDir
	}
	return workspacefs.OpenBelow(k.dir.path, area.path)
}
