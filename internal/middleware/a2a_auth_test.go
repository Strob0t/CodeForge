package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestA2AAuth_ValidKey(t *testing.T) {
	handler := A2AAuth([]string{"secret-key"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trust := A2ATrustFromContext(r.Context())
		if trust != A2ATrustPartial {
			t.Errorf("expected partial trust, got %s", trust)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestA2AAuth_MissingHeader(t *testing.T) {
	handler := A2AAuth([]string{"secret-key"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestA2AAuth_InvalidKey(t *testing.T) {
	handler := A2AAuth([]string{"secret-key"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestA2AAuth_OpenMode(t *testing.T) {
	handler := A2AAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trust := A2ATrustFromContext(r.Context())
		if trust != A2ATrustUntrusted {
			t.Errorf("expected untrusted trust, got %s", trust)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestA2AAuth_MalformedHeader(t *testing.T) {
	handler := A2AAuth([]string{"secret-key"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	req.Header.Set("Authorization", "Basic secret-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestA2AAuth_MultipleValidKeys(t *testing.T) {
	handler := A2AAuth([]string{"key-1", "key-2", "key-3"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	req.Header.Set("Authorization", "Bearer key-2")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestA2AAuth_ContextRoundtrip(t *testing.T) {
	handler := A2AAuth([]string{"key-1"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trust := A2ATrustFromContext(r.Context())
		if trust != A2ATrustPartial {
			t.Errorf("expected partial, got %s", trust)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	req.Header.Set("Authorization", "Bearer key-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
