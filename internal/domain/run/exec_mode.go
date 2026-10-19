package run

import (
	"errors"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// ErrExecModeUnavailable marks an execution mode that is defined but cannot be
// used yet. Errors carrying it also wrap domain.ErrValidation (HTTP 400).
var ErrExecModeUnavailable = errors.New("execution mode is not available yet")

// CheckAvailable reports whether a run may start in mode m. Only mount (and the
// empty mode, which defaults to mount) is available. Sandbox and hybrid fail
// closed: the worker still runs every tool as a local process, so they would
// claim an isolation that does not exist (KI-13). Unknown modes are invalid.
func (m ExecMode) CheckAvailable() error {
	switch m {
	case "", ExecModeMount:
		return nil
	case ExecModeSandbox, ExecModeHybrid:
		return fmt.Errorf("%w: %s %w: tools would run without isolation (KI-13), use %q instead",
			domain.ErrValidation, m, ErrExecModeUnavailable, ExecModeMount)
	default:
		return fmt.Errorf("%w: invalid exec_mode %q", domain.ErrValidation, m)
	}
}
