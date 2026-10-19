package main

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
)

// TestA2AKeys_OnlyWhenEnabled (S2-G fix, 11): the A2A keys are parsed only
// for an enabled A2A server; config load validates them only then too, so a
// bad entry of a disabled server must not stop the Go Core.
func TestA2AKeys_OnlyWhenEnabled(t *testing.T) {
	bad := config.A2A{APIKeys: []string{""}}
	if keys, err := a2aKeys(&bad); err != nil || keys != nil {
		t.Fatalf("a2aKeys(disabled) = %v, %v; want nil, nil", keys, err)
	}
	bad.Enabled = true
	if _, err := a2aKeys(&bad); err == nil {
		t.Fatal("a2aKeys(enabled, empty key) succeeded")
	}
	good := config.A2A{Enabled: true, APIKeys: []string{"key"}}
	if keys, err := a2aKeys(&good); err != nil || len(keys) != 1 {
		t.Fatalf("a2aKeys(enabled) = %v, %v; want one key", keys, err)
	}
}
