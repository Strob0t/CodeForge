package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicName(t *testing.T) {
	p := NewAnthropicProvider()
	if got := p.Name(); got != "anthropic" {
		t.Fatalf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestAnthropicEnvVarName(t *testing.T) {
	p := NewAnthropicProvider()
	if got := p.EnvVarName(); got != "ANTHROPIC_API_KEY" {
		t.Fatalf("EnvVarName() = %q, want %q", got, "ANTHROPIC_API_KEY")
	}
}

func TestAnthropicDeviceFlowStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/oauth/device/code" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected Content-Type: %s", ct)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["client_id"] != "codeforge" {
			t.Fatalf("unexpected client_id: %s", body["client_id"])
		}
		if body["scope"] != "user:inference" {
			t.Fatalf("unexpected scope: %s", body["scope"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCode{
			DeviceCode:      "dev-code-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://console.anthropic.com/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
	p.deviceCodeURL = srv.URL + "/v1/oauth/device/code"

	dc, err := p.DeviceFlowStart(context.Background())
	if err != nil {
		t.Fatalf("DeviceFlowStart: %v", err)
	}
	if dc.DeviceCode != "dev-code-123" {
		t.Fatalf("DeviceCode = %q, want %q", dc.DeviceCode, "dev-code-123")
	}
	if dc.UserCode != "ABCD-1234" {
		t.Fatalf("UserCode = %q, want %q", dc.UserCode, "ABCD-1234")
	}
	if dc.VerificationURI != "https://console.anthropic.com/device" {
		t.Fatalf("VerificationURI = %q", dc.VerificationURI)
	}
	if dc.ExpiresIn != 900 {
		t.Fatalf("ExpiresIn = %d, want 900", dc.ExpiresIn)
	}
	if dc.Interval != 5 {
		t.Fatalf("Interval = %d, want 5", dc.Interval)
	}
}

func TestAnthropicDeviceFlowStart_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_client"}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
	p.deviceCodeURL = srv.URL + "/v1/oauth/device/code"

	_, err := p.DeviceFlowStart(context.Background())
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}

func TestAnthropicDeviceFlowPoll(t *testing.T) {
	tests := []struct {
		name      string
		respCode  int
		respBody  map[string]string
		wantErr   error
		wantToken string
	}{
		{
			name:     "success",
			respCode: http.StatusOK,
			respBody: map[string]string{
				"access_token":  "at-123",
				"refresh_token": "rt-456",
				"token_type":    "bearer",
			},
			wantToken: "at-123",
		},
		{
			name:     "authorization_pending",
			respCode: http.StatusBadRequest,
			respBody: map[string]string{"error": "authorization_pending"},
			wantErr:  ErrAuthPending,
		},
		{
			name:     "slow_down",
			respCode: http.StatusBadRequest,
			respBody: map[string]string{"error": "slow_down"},
			wantErr:  ErrSlowDown,
		},
		{
			name:     "expired_token",
			respCode: http.StatusBadRequest,
			respBody: map[string]string{"error": "expired_token"},
			wantErr:  ErrExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/v1/oauth/token" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}

				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["grant_type"] != "urn:ietf:params:oauth:grant-type:device_code" {
					t.Fatalf("unexpected grant_type: %s", body["grant_type"])
				}
				if body["device_code"] != "dev-code-123" {
					t.Fatalf("unexpected device_code: %s", body["device_code"])
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.respCode)
				_ = json.NewEncoder(w).Encode(tt.respBody)
			}))
			defer srv.Close()

			p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
			p.tokenURL = srv.URL + "/v1/oauth/token"

			tok, err := p.DeviceFlowPoll(context.Background(), "dev-code-123")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.AccessToken != tt.wantToken {
				t.Fatalf("AccessToken = %q, want %q", tok.AccessToken, tt.wantToken)
			}
		})
	}
}

func TestAnthropicExchangeForAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/oauth/claude_cli/create_api_key" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer at-123" {
			t.Fatalf("unexpected Authorization: %s", auth)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "codeforge" {
			t.Fatalf("unexpected name: %s", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api_key": "sk-ant-test-key-789",
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
	p.apiKeyURL = srv.URL + "/api/oauth/claude_cli/create_api_key"

	key, err := p.ExchangeForAPIKey(context.Background(), &Token{
		AccessToken: "at-123",
		TokenType:   "bearer",
	})
	if err != nil {
		t.Fatalf("ExchangeForAPIKey: %v", err)
	}
	if key != "sk-ant-test-key-789" {
		t.Fatalf("key = %q, want %q", key, "sk-ant-test-key-789")
	}
}

func TestAnthropicExchangeForAPIKey_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "invalid_token"}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
	p.apiKeyURL = srv.URL + "/api/oauth/claude_cli/create_api_key"

	_, err := p.ExchangeForAPIKey(context.Background(), &Token{
		AccessToken: "bad-token",
		TokenType:   "bearer",
	})
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
}

func TestAnthropicDeviceFlowPoll_UnknownError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server_error"})
	}))
	defer srv.Close()

	p := NewAnthropicProvider(WithHTTPClient(srv.Client()))
	p.tokenURL = srv.URL + "/v1/oauth/token"

	_, err := p.DeviceFlowPoll(context.Background(), "dev-code-123")
	if err == nil {
		t.Fatal("expected error for unknown error code")
	}
	if errors.Is(err, ErrAuthPending) || errors.Is(err, ErrSlowDown) || errors.Is(err, ErrExpired) {
		t.Fatalf("should not match known errors, got: %v", err)
	}
}

// Verify the interface is satisfied at compile time.
var _ SubscriptionProvider = (*AnthropicProvider)(nil)
