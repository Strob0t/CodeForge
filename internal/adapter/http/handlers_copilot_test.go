package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const copilotBearer = "tid=copilot-bearer-SECRET;exp=1"

// fakeExchanger hands out a Copilot bearer token and counts the exchanges.
type fakeExchanger struct {
	calls atomic.Int32
	err   error
}

func (f *fakeExchanger) ExchangeToken(context.Context) (string, time.Time, error) {
	f.calls.Add(1)
	if f.err != nil {
		return "", time.Time{}, f.err
	}
	return copilotBearer, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), nil
}

func copilotRouter(ex *fakeExchanger) http.Handler {
	return newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.TokenExchanger = ex })
}

// TestCopilotExchange_NeverReturnsTheToken: the exchanged token is the
// platform's GitHub Copilot credential; no client gets it (KI-80). Only a
// platform admin may trigger the exchange, and gets its status and expiry.
func TestCopilotExchange_NeverReturnsTheToken(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	tests := []struct {
		name       string
		user       *user.User
		wantStatus int
	}{
		{"platform admin", &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}, http.StatusOK},
		{"admin of another tenant", &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: otherTenant}, http.StatusForbidden},
		{"editor of the default tenant", &user.User{ID: "de", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}, http.StatusForbidden},
		{"viewer of the default tenant", &user.User{ID: "dv", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}, http.StatusForbidden},
		{"viewer of another tenant", &user.User{ID: "tv", Role: user.RoleViewer, TenantID: otherTenant}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := &fakeExchanger{}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/copilot/exchange", http.NoBody)
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tt.user))
			w := httptest.NewRecorder()
			copilotRouter(ex).ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "copilot-bearer") {
				t.Fatalf("response contains the Copilot token: %s", w.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if ex.calls.Load() != 0 {
					t.Fatal("a forbidden request exchanged the token")
				}
				return
			}
			body := w.Body.String()
			if !strings.Contains(body, `"status":"ok"`) || !strings.Contains(body, `"expires_at":"2026-09-30T12:00:00Z"`) {
				t.Fatalf("body = %s, want status and expiry", body)
			}
			if strings.Contains(body, `"token"`) {
				t.Fatalf("body has a token field: %s", body)
			}
		})
	}
}

func TestCopilotExchange_FailureHidesDetails(t *testing.T) {
	ex := &fakeExchanger{err: errors.New("token exchange failed (HTTP 401): oauth " + copilotBearer)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/copilot/exchange", http.NoBody)
	w := httptest.NewRecorder()
	copilotRouter(ex).ServeHTTP(w, req) // the injected test user is a platform admin

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if strings.Contains(w.Body.String(), "copilot-bearer") || strings.Contains(w.Body.String(), "401") {
		t.Fatalf("error details reach the client: %s", w.Body.String())
	}
}

// TestListLLMModels_RedactsCredentials: every user may list the models, so
// credentials in their LiteLLM parameters (a registered Copilot token, API
// keys, cloud secrets, auth headers) are not passed on.
func TestListLLMModels_RedactsCredentials(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"model_name":"copilot/gpt-4","model_info":{"id":"dep-1"},"litellm_params":{
			"model":"openai/gpt-4","api_base":"https://api.githubcopilot.com","api_key":"` + copilotBearer + `",
			"aws_secret_access_key":"aws-SECRET","vertex_credentials":"vertex-SECRET",
			"extra_headers":{"Authorization":"Bearer hdr-SECRET","Copilot-Integration-Id":"vscode-chat"}}}]}`))
	}))
	defer llmSrv.Close()
	r := newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), llmSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/models", http.NoBody)
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(),
		&user.User{ID: "v", Role: user.RoleViewer, TenantID: "11111111-2222-3333-4444-555555555555"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, secret := range []string{"copilot-bearer", "aws-SECRET", "vertex-SECRET", "hdr-SECRET"} {
		if strings.Contains(body, secret) {
			t.Errorf("model list exposes %q: %s", secret, body)
		}
	}
	for _, kept := range []string{`"model":"openai/gpt-4"`, `"api_base":"https://api.githubcopilot.com"`, `"Copilot-Integration-Id":"vscode-chat"`, `"model_name":"copilot/gpt-4"`} {
		if !strings.Contains(body, kept) {
			t.Errorf("model list lost %s: %s", kept, body)
		}
	}
}
