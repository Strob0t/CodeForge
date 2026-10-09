package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
)

// newFakeDockerSandbox returns a SandboxService whose docker CLI is a script that
// records its arguments (one per line) instead of talking to a Docker daemon.
func newFakeDockerSandbox(t *testing.T, cfg SandboxConfig) (svc *SandboxService, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	script := filepath.Join(dir, "docker")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\necho fake-container-id\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil { //nolint:gosec // G306: test fixture must be executable
		t.Fatalf("write fake docker: %v", err)
	}
	svc = NewSandboxService(cfg)
	svc.docker = script
	return svc, argsFile
}

type sandboxCreateFunc func(s *SandboxService, ctx context.Context, runID, workspacePath string, overrides ...resource.Limits) (*Sandbox, error)

var sandboxCreators = []struct {
	name   string
	create sandboxCreateFunc
}{
	{"Create", (*SandboxService).Create},
	{"CreateHybrid", (*SandboxService).CreateHybrid},
}

func TestSandboxCreate_CPUsFlagIsDecimal(t *testing.T) {
	tests := []struct {
		name     string
		quota    int
		override resource.Limits
		want     string
	}{
		{name: "one CPU", quota: 1000, want: "--cpus=1"},
		{name: "half CPU", quota: 500, want: "--cpus=0.5"},
		{name: "one and a half CPUs", quota: 1500, want: "--cpus=1.5"},
		{name: "smallest quota", quota: 1, want: "--cpus=0.001"},
		{name: "quarter CPU from override", quota: 1000, override: resource.Limits{CPUQuota: 250}, want: "--cpus=0.25"},
		{name: "override capped at 4x default", quota: 500, override: resource.Limits{CPUQuota: 9000}, want: "--cpus=2"},
	}
	for _, c := range sandboxCreators {
		for _, tc := range tests {
			t.Run(c.name+"/"+tc.name, func(t *testing.T) {
				cfg := DefaultSandboxConfig()
				cfg.CPUQuota = tc.quota
				svc, argsFile := newFakeDockerSandbox(t, cfg)

				if _, err := c.create(svc, context.Background(), "run-cpus", "/workspace", tc.override); err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				raw, err := os.ReadFile(argsFile) //nolint:gosec // G304: path is the test's own temp file
				if err != nil {
					t.Fatalf("read recorded docker args: %v", err)
				}
				args := strings.Split(strings.TrimSpace(string(raw)), "\n")
				if !slices.Contains(args, tc.want) {
					t.Fatalf("docker args %q do not contain %q", args, tc.want)
				}
			})
		}
	}
}

func TestSandboxCreate_RejectsNonPositiveCPUQuota(t *testing.T) {
	for _, c := range sandboxCreators {
		for _, quota := range []int{0, -1, -1000} {
			t.Run(fmt.Sprintf("%s/quota=%d", c.name, quota), func(t *testing.T) {
				cfg := DefaultSandboxConfig()
				cfg.CPUQuota = quota
				svc, argsFile := newFakeDockerSandbox(t, cfg)

				sb, err := c.create(svc, context.Background(), "run-cpus", "/workspace")
				if err == nil {
					t.Fatalf("%s with cpu_quota %d: expected an error, got sandbox %+v", c.name, quota, sb)
				}
				if !strings.Contains(err.Error(), "cpu_quota") {
					t.Fatalf("error %q should name cpu_quota", err)
				}
				if _, statErr := os.Stat(argsFile); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("docker must not be invoked for an invalid cpu_quota (stat err: %v)", statErr)
				}
				if _, ok := svc.Get("run-cpus"); ok {
					t.Fatal("no sandbox may be registered for a rejected create")
				}
			})
		}
	}
}
