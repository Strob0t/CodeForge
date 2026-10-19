package service

import (
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// projectExecModeKey is the project config key holding the default execution
// mode for the project's runs and agentic conversations.
const projectExecModeKey = "execution_mode"

// resolveExecMode returns the execution mode a run or agentic conversation starts
// in: the requested mode, else the project's configured default, else mount.
// It fails closed for unknown values and for modes that cannot isolate tool
// execution yet (see run.ExecMode.CheckAvailable).
func resolveExecMode(requested run.ExecMode, proj *project.Project) (run.ExecMode, error) {
	mode := requested
	if mode == "" {
		mode = run.ExecMode(proj.Config[projectExecModeKey])
	}
	if err := mode.CheckAvailable(); err != nil {
		return "", err
	}
	if mode == "" {
		return run.ExecModeMount, nil
	}
	return mode, nil
}

// checkBenchmarkExecMode applies the run exec-mode gate to benchmarks: agent
// benchmarks execute their tools in the worker exactly like a run does.
func checkBenchmarkExecMode(mode benchmark.ExecMode) error {
	return run.ExecMode(mode).CheckAvailable()
}
