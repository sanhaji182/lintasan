package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestMiddleware_SkipsHealthAndLogin(t *testing.T) {
	m := newUserMgr(t)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Health should pass through without auth
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /health, got %d", w.Code)
	}

	// Login should pass through without auth
	req2 := httptest.NewRequest("POST", "/api/auth/login", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/auth/login, got %d", w2.Code)
	}
}

func TestMiddleware_RejectsNoAuth(t *testing.T) {
	m := newUserMgr(t)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/protected", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no auth, got %d", w.Code)
	}
}

func TestMiddleware_RejectsBadBearer(t *testing.T) {
	m := newUserMgr(t)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "NotBearer token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad auth scheme, got %d", w.Code)
	}
}

func TestMiddleware_AcceptsCookieAuth(t *testing.T) {
	m := newUserMgr(t)
	_, _ = m.CreateUser("cookieuser", "pass123", "user")
	token, _, err := m.Authenticate("cookieuser", "pass123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil {
			t.Error("expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "lintasan_token", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for cookie auth, got %d", w.Code)
	}
}

func TestMiddleware_RejectsInvalidToken(t *testing.T) {
	m := newUserMgr(t)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin-only", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, &User{Role: "admin"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", w.Code)
	}
}

func TestRequireAdmin_RejectsUser(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin-only", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, &User{Role: "user"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestRequireAdmin_RejectsNoUser(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin-only", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for no user, got %d", w.Code)
	}
}

func TestCORSHeaders_SetsHeaders(t *testing.T) {
	handler := CORSHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS Allow-Origin *")
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("expected CORS Allow-Methods")
	}
}

func TestCORSHeaders_HandlesPreflight(t *testing.T) {
	handler := CORSHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS preflight, got %d", w.Code)
	}
}

func TestGetUser_ReturnsNilForNoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	user := GetUser(req)
	if user != nil {
		t.Error("expected nil user when no context")
	}
}

func TestLoginResponse(t *testing.T) {
	user := &User{
		ID:                 "user_123",
		Username:           "testuser",
		Role:               "admin",
		MustChangePassword: true,
	}
	resp := NewLoginResponse("token-abc", user)
	if resp.Token != "token-abc" {
		t.Errorf("expected token 'token-abc', got '%s'", resp.Token)
	}
	if resp.User.Username != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", resp.User.Username)
	}
	if resp.User.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", resp.User.Role)
	}
	if !resp.User.MustChangePassword {
		t.Error("expected MustChangePassword=true")
	}
}

func TestAuthHandler_Login_InvalidBody(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleLogin().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
