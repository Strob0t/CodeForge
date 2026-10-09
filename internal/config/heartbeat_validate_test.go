package config

import (
	"strings"
	"testing"
	"time"
)

// A heartbeat timeout not longer than the heartbeat interval ends every
// healthy run between two heartbeats (S2-F review, F11): it is refused at
// load, as is an interval the worker cannot send (below 1 s).
func TestValidate_Heartbeat(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		timeout  time.Duration
		wantErr  string
	}{
		{name: "defaults", interval: 30 * time.Second, timeout: 120 * time.Second},
		{name: "default interval", interval: 0, timeout: 31 * time.Second},
		{name: "timeout disabled", interval: 30 * time.Second, timeout: 0},
		{name: "timeout equal to the interval", interval: 30 * time.Second, timeout: 30 * time.Second, wantErr: "runtime.heartbeat_timeout"},
		{name: "timeout below the default interval", interval: 0, timeout: 20 * time.Second, wantErr: "runtime.heartbeat_timeout"},
		{name: "interval below 1s", interval: 500 * time.Millisecond, timeout: time.Minute, wantErr: "runtime.heartbeat_interval"},
		{name: "negative interval", interval: -time.Second, timeout: time.Minute, wantErr: "runtime.heartbeat_interval"},
		{name: "negative timeout", interval: 30 * time.Second, timeout: -time.Second, wantErr: "runtime.heartbeat_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.Runtime.HeartbeatInterval = tt.interval
			cfg.Runtime.HeartbeatTimeout = tt.timeout
			err := validate(&cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v, want no error", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate() = %v, want an error naming %s", err, tt.wantErr)
			}
		})
	}
}

func TestRuntime_WorkerHeartbeatInterval(t *testing.T) {
	if got := (&Runtime{}).WorkerHeartbeatInterval(); got != DefaultWorkerHeartbeatInterval {
		t.Errorf("unset interval = %s, want the default %s", got, DefaultWorkerHeartbeatInterval)
	}
	if got := (&Runtime{HeartbeatInterval: 10 * time.Second}).WorkerHeartbeatInterval(); got != 10*time.Second {
		t.Errorf("configured interval = %s, want 10s", got)
	}
}

// TestValidate_TaskAcceptTimeout: runtime.task_accept_timeout (how long a
// dispatched backend task may wait for a worker) defaults to 1h; 0 turns the
// check off, a negative value is refused.
func TestValidate_TaskAcceptTimeout(t *testing.T) {
	if got := Defaults().Runtime.TaskAcceptTimeout; got != time.Hour {
		t.Fatalf("default task_accept_timeout = %s, want 1h", got)
	}
	for _, tt := range []struct {
		timeout time.Duration
		wantErr bool
	}{{0, false}, {time.Minute, false}, {-time.Second, true}} {
		cfg := Defaults()
		cfg.Auth.JWTSecret = strongTestSecret
		cfg.Runtime.TaskAcceptTimeout = tt.timeout
		err := validate(&cfg)
		if tt.wantErr != (err != nil) || (err != nil && !strings.Contains(err.Error(), "runtime.task_accept_timeout")) {
			t.Fatalf("validate(task_accept_timeout %s) = %v, wantErr %v", tt.timeout, err, tt.wantErr)
		}
	}
}
