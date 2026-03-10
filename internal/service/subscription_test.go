package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/auth"
)

// fakeProvider implements auth.SubscriptionProvider for testing.
type fakeProvider struct {
	name        string
	envVar      string
	startErr    error
	pollResult  *auth.Token
	pollErr     error
	exchangeRv  string
	exchangeErr error
}

func (f *fakeProvider) Name() string       { return f.name }
func (f *fakeProvider) EnvVarName() string { return f.envVar }

func (f *fakeProvider) DeviceFlowStart(_ context.Context) (*auth.DeviceCode, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &auth.DeviceCode{
		DeviceCode:      "test-device-code",
		UserCode:        "TEST-1234",
		VerificationURI: "https://example.com/device",
		ExpiresIn:       300,
		Interval:        0, // use minInterval from service
	}, nil
}

func (f *fakeProvider) DeviceFlowPoll(_ context.Context, _ string) (*auth.Token, error) {
	if f.pollErr != nil {
		return nil, f.pollErr
	}
	return f.pollResult, nil
}

func (f *fakeProvider) ExchangeForAPIKey(_ context.Context, _ *auth.Token) (string, error) {
	if f.exchangeErr != nil {
		return "", f.exchangeErr
	}
	return f.exchangeRv, nil
}

func TestSubscriptionService_ListProviders(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TEST_KEY=sk-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSubscriptionService(envPath,
		&fakeProvider{name: "test_connected", envVar: "TEST_KEY"},
		&fakeProvider{name: "test_disconnected", envVar: "MISSING_KEY"},
	)

	infos := svc.ListProviders()
	if len(infos) != 2 {
		t.Fatalf("got %d providers, want 2", len(infos))
	}

	byName := make(map[string]ProviderInfo)
	for _, info := range infos {
		byName[info.Name] = info
	}

	if !byName["test_connected"].Connected {
		t.Error("test_connected should be connected")
	}
	if byName["test_disconnected"].Connected {
		t.Error("test_disconnected should not be connected")
	}
}

func TestSubscriptionService_StartConnect(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	provider := &fakeProvider{
		name:       "test_provider",
		envVar:     "TEST_API_KEY",
		pollResult: &auth.Token{AccessToken: "test-token"},
		exchangeRv: "sk-test-api-key",
	}

	svc := NewSubscriptionService(envPath, provider)
	svc.minInterval = 100 * time.Millisecond // Speed up for testing.

	dc, err := svc.StartConnect(context.Background(), "test_provider")
	if err != nil {
		t.Fatalf("StartConnect: %v", err)
	}
	if dc.UserCode != "TEST-1234" {
		t.Errorf("UserCode = %q, want %q", dc.UserCode, "TEST-1234")
	}

	// Wait for the background polling to complete.
	time.Sleep(500 * time.Millisecond)

	// After flow completion and cleanup, GetStatus falls through to envWriter check.
	status := svc.GetStatus("test_provider")
	if status.Status != "complete" {
		t.Errorf("status = %q, want %q (error: %s)", status.Status, "complete", status.Error)
	}

	// Verify the API key was written to .env.
	val, err := svc.envWriter.Get("TEST_API_KEY")
	if err != nil {
		t.Fatalf("envWriter.Get: %v", err)
	}
	if val != "sk-test-api-key" {
		t.Errorf("env value = %q, want %q", val, "sk-test-api-key")
	}
}

func TestSubscriptionService_StartConnect_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	svc := NewSubscriptionService(envPath)

	_, err := svc.StartConnect(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestSubscriptionService_GetStatus_NoFlow(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	svc := NewSubscriptionService(envPath,
		&fakeProvider{name: "test", envVar: "TEST_KEY"},
	)

	status := svc.GetStatus("test")
	if status.Status != "error" {
		t.Errorf("status = %q, want %q", status.Status, "error")
	}
}

func TestSubscriptionService_GetStatus_AlreadyConnected(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TEST_KEY=existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSubscriptionService(envPath,
		&fakeProvider{name: "test", envVar: "TEST_KEY"},
	)

	status := svc.GetStatus("test")
	if status.Status != "complete" {
		t.Errorf("status = %q, want %q", status.Status, "complete")
	}
}

func TestSubscriptionService_Disconnect(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TEST_KEY=sk-test\nOTHER=keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSubscriptionService(envPath,
		&fakeProvider{name: "test", envVar: "TEST_KEY"},
	)

	if err := svc.Disconnect("test"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	has, _ := svc.envWriter.Has("TEST_KEY")
	if has {
		t.Error("TEST_KEY should be deleted after disconnect")
	}

	other, _ := svc.envWriter.Get("OTHER")
	if other != "keep" {
		t.Errorf("OTHER = %q, want %q", other, "keep")
	}
}

func TestSubscriptionService_Disconnect_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	svc := NewSubscriptionService(envPath)

	err := svc.Disconnect("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestSubscriptionService_StartConnect_PendingFlow(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	// Provider that stays pending (returns ErrAuthPending).
	provider := &fakeProvider{
		name:    "slow",
		envVar:  "SLOW_KEY",
		pollErr: auth.ErrAuthPending,
	}

	svc := NewSubscriptionService(envPath, provider)

	_, err := svc.StartConnect(context.Background(), "slow")
	if err != nil {
		t.Fatalf("StartConnect: %v", err)
	}

	// Give polling goroutine a moment to start.
	time.Sleep(100 * time.Millisecond)

	status := svc.GetStatus("slow")
	if status.Status != "pending" {
		t.Errorf("status = %q, want %q", status.Status, "pending")
	}
}
