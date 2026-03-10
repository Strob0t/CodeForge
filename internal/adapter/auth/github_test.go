package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubName(t *testing.T) {
	p := NewGitHubProvider()
	if got := p.Name(); got != "github_copilot" {
		t.Fatalf("Name() = %q, want %q", got, "github_copilot")
	}
}

func TestGitHubEnvVarName(t *testing.T) {
	p := NewGitHubProvider()
	if got := p.EnvVarName(); got != "GITHUB_TOKEN" {
		t.Fatalf("EnvVarName() = %q, want %q", got, "GITHUB_TOKEN")
	}
}

func TestGitHubDeviceFlowStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/login/device/code" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Fatalf("unexpected Accept: %s", accept)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["client_id"] == "" {
			t.Fatal("expected non-empty client_id")
		}
		if body["scope"] != "copilot" {
			t.Fatalf("unexpected scope: %s", body["scope"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCode{
			DeviceCode:      "gh-dev-code-456",
			UserCode:        "WXYZ-9876",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer srv.Close()

	p := NewGitHubProvider(WithHTTPClient(srv.Client()))
	p.deviceCodeURL = srv.URL + "/login/device/code"

	dc, err := p.DeviceFlowStart(context.Background())
	if err != nil {
		t.Fatalf("DeviceFlowStart: %v", err)
	}
	if dc.DeviceCode != "gh-dev-code-456" {
		t.Fatalf("DeviceCode = %q, want %q", dc.DeviceCode, "gh-dev-code-456")
	}
	if dc.UserCode != "WXYZ-9876" {
		t.Fatalf("UserCode = %q, want %q", dc.UserCode, "WXYZ-9876")
	}
	if dc.ExpiresIn != 900 {
		t.Fatalf("ExpiresIn = %d, want 900", dc.ExpiresIn)
	}
}

func TestGitHubDeviceFlowStart_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error": "invalid_scope"}`))
	}))
	defer srv.Close()

	p := NewGitHubProvider(WithHTTPClient(srv.Client()))
	p.deviceCodeURL = srv.URL + "/login/device/code"

	_, err := p.DeviceFlowStart(context.Background())
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}

func TestGitHubDeviceFlowPoll(t *testing.T) {
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
				"access_token": "gho_abc123",
				"token_type":   "bearer",
			},
			wantToken: "gho_abc123",
		},
		{
			name:     "authorization_pending",
			respCode: http.StatusOK,
			respBody: map[string]string{"error": "authorization_pending"},
			wantErr:  ErrAuthPending,
		},
		{
			name:     "slow_down",
			respCode: http.StatusOK,
			respBody: map[string]string{"error": "slow_down"},
			wantErr:  ErrSlowDown,
		},
		{
			name:     "expired_token",
			respCode: http.StatusOK,
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
				if r.URL.Path != "/login/oauth/access_token" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				if accept := r.Header.Get("Accept"); accept != "application/json" {
					t.Fatalf("unexpected Accept: %s", accept)
				}

				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["grant_type"] != "urn:ietf:params:oauth:grant-type:device_code" {
					t.Fatalf("unexpected grant_type: %s", body["grant_type"])
				}
				if body["device_code"] != "gh-dev-code-456" {
					t.Fatalf("unexpected device_code: %s", body["device_code"])
				}
				if body["client_id"] == "" {
					t.Fatal("expected non-empty client_id")
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.respCode)
				_ = json.NewEncoder(w).Encode(tt.respBody)
			}))
			defer srv.Close()

			p := NewGitHubProvider(WithHTTPClient(srv.Client()))
			p.tokenURL = srv.URL + "/login/oauth/access_token"

			tok, err := p.DeviceFlowPoll(context.Background(), "gh-dev-code-456")

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

func TestGitHubExchangeForAPIKey(t *testing.T) {
	p := NewGitHubProvider()
	tok := &Token{
		AccessToken: "gho_abc123",
		TokenType:   "bearer",
	}

	key, err := p.ExchangeForAPIKey(context.Background(), tok)
	if err != nil {
		t.Fatalf("ExchangeForAPIKey: %v", err)
	}
	if key != "gho_abc123" {
		t.Fatalf("key = %q, want %q", key, "gho_abc123")
	}
}

func TestGitHubDeviceFlowPoll_UnknownError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	}))
	defer srv.Close()

	p := NewGitHubProvider(WithHTTPClient(srv.Client()))
	p.tokenURL = srv.URL + "/login/oauth/access_token"

	_, err := p.DeviceFlowPoll(context.Background(), "gh-dev-code-456")
	if err == nil {
		t.Fatal("expected error for unknown error code")
	}
	if errors.Is(err, ErrAuthPending) || errors.Is(err, ErrSlowDown) || errors.Is(err, ErrExpired) {
		t.Fatalf("should not match known errors, got: %v", err)
	}
}

func TestGitHubDeviceFlowPoll_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	p := NewGitHubProvider(WithHTTPClient(srv.Client()))
	p.tokenURL = srv.URL + "/login/oauth/access_token"

	_, err := p.DeviceFlowPoll(context.Background(), "gh-dev-code-456")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

// Verify the interface is satisfied at compile time.
var _ SubscriptionProvider = (*GitHubProvider)(nil)
