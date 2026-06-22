package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestAuthHandler_Me_Authenticated(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("meuser", "pass123", "admin")
	_, _, _ = mgr.Authenticate("meuser", "pass123")

	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID:       "user_meuser",
		Username: "meuser",
		Role:     "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleMe().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_HandleMe_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/me", nil)
	w := httptest.NewRecorder()
	h.HandleMe().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAuthHandler_HandleListUsers_AuthenticatedAdmin(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("admin1", "pass", "admin")
	_, _ = mgr.CreateUser("user1", "pass", "user")

	h := NewAuthHandler(mgr)
	req := httptest.NewRequest("GET", "/api/auth/users", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "admin1", Username: "admin1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleListUsers().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_HandleListUsers_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/users", nil)
	w := httptest.NewRecorder()
	h.HandleListUsers().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAuthHandler_HandleCreateUser_AuthenticatedAdmin(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("existing-admin", "pass", "admin")

	h := NewAuthHandler(mgr)
	body := `{"username":"newguy","password":"newpass123","role":"user"}`
	req := httptest.NewRequest("POST", "/api/auth/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "existing-admin", Username: "existing-admin", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_HandleCreateUser_EmptyFields(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/users", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "admin1", Username: "admin1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty fields, got %d", w.Code)
	}
}

func TestAuthHandler_HandleCreateUser_InvalidBody(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/users", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "admin1", Username: "admin1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestAuthHandler_HandleCreateUser_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/users", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "a1", Username: "a1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAuthHandler_HandleLogout_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	h.HandleLogout().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthHandler_HandleChangePassword_Authenticated(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	u, _ := mgr.CreateUser("changepw", "current-pass-123", "user")

	h := NewAuthHandler(mgr)
	body := `{"current_password":"current-pass-123","new_password":"new-longer-pass-456"}`
	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: u.ID, Username: "changepw", Role: "user",
	}))
	w := httptest.NewRecorder()
	h.HandleChangePassword().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_HandleChangePassword_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/change-password", nil)
	w := httptest.NewRecorder()
	h.HandleChangePassword().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAuthHandler_HandleChangePassword_InvalidBody(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "u1", Username: "u1", Role: "user",
	}))
	w := httptest.NewRecorder()
	h.HandleChangePassword().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUserManager_ChangePassword_UserNotFound(t *testing.T) {
	m := newUserMgr(t)
	err := m.ChangePassword("nonexistent-id", "old", "new-password-123")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_DeleteUser_NotFoundViaRows(t *testing.T) {
	conn, _ := sql.Open("sqlite3", ":memory:")
	defer conn.Close()
	mgr := NewUserManager(conn, "secret")
	err := mgr.DeleteUser("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUserManager_UpdateUserRole_UserNotFound(t *testing.T) {
	m := newUserMgr(t)
	err := m.UpdateUserRole("nonexistent", "admin")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUserManager_SeedAdmin_AlreadySeeded(t *testing.T) {
	m := newUserMgr(t)
	_, _ = m.SeedAdmin("admin1")
	pw, err := m.SeedAdmin("admin2")
	if err != nil {
		t.Fatalf("unexpected error on second seed: %v", err)
	}
	if pw != "" {
		t.Errorf("expected empty string for already-seeded")
	}
}

func TestUserManager_ListUsers_EmptyTable(t *testing.T) {
	m := newUserMgr(t)
	users, err := m.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestNewLoginResponse_Fields(t *testing.T) {
	user := &User{
		ID:                 "u1",
		Username:           "testuser",
		Role:               "admin",
		MustChangePassword: true,
	}
	resp := NewLoginResponse("", user)
	if resp.Token != "" {
		t.Errorf("expected empty token")
	}
	if resp.User.Username != "testuser" {
		t.Errorf("expected username 'testuser'")
	}
}

func TestValidateJWT_InvalidPayload(t *testing.T) {
	parts := []string{"eyJhbG...VCJ9", "!!!invalid!!!", "signature"}
	_, err := ValidateJWT(strings.Join(parts, "."), "secret")
	if err == nil {
		t.Error("expected error for invalid payload encoding")
	}
}

func TestValidateJWT_InvalidClaimsJSON(t *testing.T) {
	parts := []string{
		base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`)),
		base64URLEncode([]byte(`not-json`)),
		"signature",
	}
	_, err := ValidateJWT(strings.Join(parts, "."), "secret")
	if err == nil {
		t.Error("expected error for invalid claims JSON")
	}
}

func TestAuthHandler_ListUsers_RBAC_NonAdminUser(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/users", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "u1", Username: "u1", Role: "user",
	}))
	w := httptest.NewRecorder()
	h.HandleListUsers().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin requesting user list, got %d", w.Code)
	}
}

func TestAuthHandler_CreateUser_DuplicateUsername(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("unique", "pass", "user")

	h := NewAuthHandler(mgr)
	body := `{"username":"unique","password":"newpass123","role":"user"}`
	req := httptest.NewRequest("POST", "/api/auth/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &User{
		ID: "a1", Username: "a1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate, got %d", w.Code)
	}
}

func TestHashPassword_ProducesValidFormat(t *testing.T) {
	hash, err := HashPassword("hello123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$sha512$200000$") {
		t.Errorf("unexpected hash format: %s", hash)
	}
}

func TestHashWithSalt_HashEmptyPassword(t *testing.T) {
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = 0x41
	}
	hash := hashWithSalt("", salt)
	if len(hash) == 0 {
		t.Error("expected non-empty hash for empty password")
	}
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	h1, _ := HashPassword("password123")
	h2, _ := HashPassword("password123")
	if h1 == h2 {
		t.Error("expected different hashes due to random salt")
	}
}
