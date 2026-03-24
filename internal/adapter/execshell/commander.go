// Package execshell implements the shell commander port using os/exec.
package execshell

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/Strob0t/CodeForge/internal/port/shell"
)

// Commander implements shell.Commander backed by os/exec.
type Commander struct{}

// New creates a new exec-backed shell commander.
func New() *Commander { return &Commander{} }

func (c *Commander) Run(ctx context.Context, dir, name string, args ...string) (*shell.Result, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // command from trusted caller
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := &shell.Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func (c *Commander) RunCombined(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // command from trusted caller
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
