// Package shell defines the port interface for shell command execution.
package shell

import "context"

// Result holds the output of a command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Commander abstracts os/exec for service layer decoupling.
type Commander interface {
	Run(ctx context.Context, dir string, name string, args ...string) (*Result, error)
	RunCombined(ctx context.Context, dir string, name string, args ...string) (string, error)
}
