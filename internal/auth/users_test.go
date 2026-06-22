package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestGenerateAndValidateJWT(t *testing.T) {
	claims := JWTClaims{
		Sub:      "user_test_123",
		Username: "testuser",
		Role:     "admin",
	}

	token, err := GenerateJWT(claims, "test-secret")
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3-part JWT, got %d parts", len(parts))
	}

	got, err := ValidateJWT(token, "test-secret")
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}
	if got.Sub != "user_test_123" {
		t.Errorf("expected sub 'user_test_123', got '%s'", got.Sub)
	}
	if got.Username != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", got.Username)
	}
	if got.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", got.Role)
	}
}

func TestValidateJWT_InvalidTokenFormat(t *testing.T) {
	_, err := ValidateJWT("not-a-valid-jwt", "secret")
	if err == nil {
		t.Fatal("expected error for invalid token format")
	}
}

func TestValidateJWT_TamperedSignature(t *testing.T) {
	token, err := GenerateJWT(JWTClaims{Sub: "user1", Exp: time.Now().Add(1 * time.Hour).Unix()}, "secret1")
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	// Validate with wrong secret
	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for tampered signature")
	}
}

func TestValidateJWT_ExpiredToken(t *testing.T) {
	claims := JWTClaims{
		Sub: "user1",
		Exp: time.Now().Add(-1 * time.Hour).Unix(), // expired
		Iat: time.Now().Add(-2 * time.Hour).Unix(),
	}
	token, err := GenerateJWT(claims, "test-secret")
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	_, err = ValidateJWT(token, "test-secret")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestGenerateJWT_DefaultExpiry(t *testing.T) {
	token, err := GenerateJWT(JWTClaims{Sub: "user1"}, "secret")
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	claims, err := ValidateJWT(token, "secret")
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}
	if claims.Exp == 0 {
		t.Fatal("expected Exp to be set to default")
	}
	if claims.Sub != "user1" {
		t.Errorf("expected sub 'user1', got '%s'", claims.Sub)
	}
}

// --- Password Hashing Tests ---

func TestHashAndVerifyPassword(t *testing.T) {
	password := "my-secure-password-123!@#"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword returned false for correct password")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Error("VerifyPassword returned true for wrong password")
	}
}

func TestVerifyPassword_InvalidFormat(t *testing.T) {
	if VerifyPassword("password", "not-a-valid-format") {
		t.Error("expected false for invalid hash format")
	}
	if VerifyPassword("password", "$sha512$100000$invalidsalt$hash") {
		t.Error("expected false for invalid hex salt")
	}
	if VerifyPassword("password", "$sha512$100000$616161$invaidhex") {
		t.Error("expected false for invalid hex hash")
	}
}

func TestBase64URLRoundTrip(t *testing.T) {
	tests := []string{
		"hello",
		"a",
		"ab",
		"abc",
		"longer test data with spaces and symbols!@#$%",
		"",
	}
	for _, s := range tests {
		encoded := base64URLEncode([]byte(s))
		decoded, err := base64URLDecode(encoded)
		if err != nil {
			t.Errorf("base64URLDecode(%q) failed: %v", encoded, err)
		}
		if string(decoded) != s {
			t.Errorf("round trip: got %q, want %q", string(decoded), s)
		}
	}
}

func TestBase64URLDecode_Invalid(t *testing.T) {
	_, err := base64URLDecode("!!!invalid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestHmacSHA256(t *testing.T) {
	sig := hmacSHA256("test data", "secret-key")
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	// Deterministic
	sig2 := hmacSHA256("test data", "secret-key")
	if sig != sig2 {
		t.Error("expected deterministic signature")
	}
	// Different key → different signature
	sig3 := hmacSHA256("test data", "different-key")
	if sig == sig3 {
		t.Error("expected different signature for different key")
	}
}

// --- UserManager Tests ---

func newUserMgr(t *testing.T) *UserManager {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewUserManager(database.Conn(), "test-secret")
}

func TestUserManager_CreateAndAuthenticate(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("alice", "alice-pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("expected username 'alice', got '%s'", u.Username)
	}
	if u.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", u.Role)
	}

	// Authenticate
	token, user, err := m.Authenticate("alice", "alice-pass")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if user.Username != "alice" {
		t.Errorf("expected authenticated user 'alice', got '%s'", user.Username)
	}
}

func TestUserManager_Authenticate_WrongPassword(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.CreateUser("bob", "bob-pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	_, _, err = m.Authenticate("bob", "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestUserManager_Authenticate_Nonexistent(t *testing.T) {
	m := newUserMgr(t)
	_, _, err := m.Authenticate("nonexistent", "pass")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_DuplicateUsername(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.CreateUser("dup", "pass1", "user")
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = m.CreateUser("dup", "pass2", "admin")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestUserManager_SeedAdmin_NoUsersExists(t *testing.T) {
	m := newUserMgr(t)
	password, err := m.SeedAdmin("admin")
	if err != nil {
		t.Fatalf("SeedAdmin failed: %v", err)
	}
	if password == "" {
		t.Fatal("expected non-empty generated password")
	}
	// Should be able to authenticate with generated password
	token, _, err := m.Authenticate("admin", password)
	if err != nil {
		t.Fatalf("authenticate with seeded admin failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestUserManager_SeedAdmin_SkipIfUsersExist(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.CreateUser("existing", "pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	password, err := m.SeedAdmin("should-not-create")
	if err != nil {
		t.Fatalf("SeedAdmin failed: %v", err)
	}
	if password != "" {
		t.Errorf("expected empty password when users already exist, got '%s'", password)
	}
}

func TestUserManager_ChangePassword(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("changeme", "old-pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.ChangePassword(u.ID, "old-pass", "new-pass-123")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}
	// Should authenticate with new password
	_, _, err = m.Authenticate("changeme", "new-pass-123")
	if err != nil {
		t.Fatalf("authenticate with new password failed: %v", err)
	}
	// Should fail with old password
	_, _, err = m.Authenticate("changeme", "old-pass")
	if err == nil {
		t.Fatal("expected error authenticating with old password after change")
	}
}

func TestUserManager_ChangePassword_WrongCurrent(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("user1", "correct-pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.ChangePassword(u.ID, "wrong-current", "new-pass")
	if err == nil {
		t.Fatal("expected error for wrong current password")
	}
}

func TestUserManager_ChangePassword_TooShort(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("user2", "long-enough-pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.ChangePassword(u.ID, "long-enough-pass", "short")
	if err == nil {
		t.Fatal("expected error for too-short new password")
	}
}

func TestUserManager_ChangePassword_SameAsOld(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("user3", "same-password-123", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.ChangePassword(u.ID, "same-password-123", "same-password-123")
	if err == nil {
		t.Fatal("expected error when new password equals current")
	}
}

func TestUserManager_UpdateUserRole(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("roleuser", "pass", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.UpdateUserRole(u.ID, "admin")
	if err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}
	// Verify
	updated, err := m.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", updated.Role)
	}
}

func TestUserManager_UpdateUserRole_Invalid(t *testing.T) {
	m := newUserMgr(t)
	u, _ := m.CreateUser("roleuser2", "pass", "user")
	err := m.UpdateUserRole(u.ID, "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestUserManager_AdminSetPassword(t *testing.T) {
	m := newUserMgr(t)
	u, err := m.CreateUser("resetme", "old-pass-123", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	err = m.AdminSetPassword(u.ID, "new-admin-set-123")
	if err != nil {
		t.Fatalf("AdminSetPassword failed: %v", err)
	}
	// Should authenticate with new password
	_, _, err = m.Authenticate("resetme", "new-admin-set-123")
	if err != nil {
		t.Fatalf("authenticate after admin reset failed: %v", err)
	}
}

func TestUserManager_AdminSetPassword_TooShort(t *testing.T) {
	m := newUserMgr(t)
	u, _ := m.CreateUser("reset2", "pass", "user")
	err := m.AdminSetPassword(u.ID, "short")
	if err == nil {
		t.Fatal("expected error for too-short password")
	}
}

func TestUserManager_AdminSetPassword_Nonexistent(t *testing.T) {
	m := newUserMgr(t)
	err := m.AdminSetPassword("nonexistent-id", "new-pass-123")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_ListUsers(t *testing.T) {
	m := newUserMgr(t)
	_, _ = m.CreateUser("alice", "pass", "admin")
	_, _ = m.CreateUser("bob", "pass", "user")
	_, _ = m.CreateUser("charlie", "pass", "user")

	users, err := m.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}
}

func TestUserManager_AdminCount(t *testing.T) {
	m := newUserMgr(t)
	if m.AdminCount() != 0 {
		t.Errorf("expected 0 admins initially, got %d", m.AdminCount())
	}
	_, _ = m.CreateUser("admin1", "pass", "admin")
	if m.AdminCount() != 1 {
		t.Errorf("expected 1 admin, got %d", m.AdminCount())
	}
	_, _ = m.CreateUser("admin2", "pass", "admin")
	if m.AdminCount() != 2 {
		t.Errorf("expected 2 admins, got %d", m.AdminCount())
	}
}

func TestUserManager_UpdateUserRole_DemoteLastAdmin(t *testing.T) {
	m := newUserMgr(t)
	admin, _ := m.CreateUser("only-admin", "pass", "admin")
	err := m.UpdateUserRole(admin.ID, "user")
	if err == nil {
		t.Fatal("expected error when demoting last admin")
	}
}

func TestUserManager_DeleteUser_NotFound(t *testing.T) {
	m := newUserMgr(t)
	err := m.DeleteUser("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_GetByID_NotFound(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.GetByID("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_GetByUsername_NotFound(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.GetByUsername("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserManager_ValidateToken_Invalid(t *testing.T) {
	m := newUserMgr(t)
	_, err := m.ValidateToken("invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestUserManager_ValidateToken_Expired(t *testing.T) {
	m := newUserMgr(t)
	// Generate expired token manually
	token, err := GenerateJWT(JWTClaims{
		Sub: "nonexistent",
		Exp: time.Now().Add(-1 * time.Hour).Unix(),
	}, "test-secret")
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}
	_, err = m.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

// --- AuthHandler Tests ---

func TestAuthHandler_Login_Success(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("testuser", "testpass", "user")

	h := NewAuthHandler(mgr)
	handler := h.HandleLogin()

	body := `{"username":"testuser","password":"testpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	_, _ = mgr.CreateUser("u", "p", "user")

	h := NewAuthHandler(mgr)
	body := `{"username":"u","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleLogin().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthHandler_Login_EmptyFields(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	tests := []string{
		`{"username":"","password":"pass"}`,
		`{"username":"user","password":""}`,
		`{}`,
	}
	for _, body := range tests {
		req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.HandleLogin().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("for body %q: expected 400, got %d", body, w.Code)
		}
	}
}

func TestAuthHandler_Login_WrongMethod(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/login", nil)
	w := httptest.NewRecorder()
	h.HandleLogin().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAuthHandler_Me_Unauthenticated(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()
	h.HandleMe().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthHandler_Logout(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	h.HandleLogout().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// Check cookie cleared
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "lintasan_token" && c.MaxAge == -1 {
			found = true
		}
	}
	if !found {
		t.Error("expected cookie to be cleared with MaxAge=-1")
	}
}

func TestAuthHandler_ListUsers_Unauthenticated(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("GET", "/api/auth/users", nil)
	w := httptest.NewRecorder()
	h.HandleListUsers().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAuthHandler_CreateUser_Unauthenticated(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/users", strings.NewReader(`{"username":"x","password":"pass"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleCreateUser().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAuthHandler_ChangePassword_Unauthenticated(t *testing.T) {
	database, _ := db.Open(":memory:")
	defer database.Close()
	mgr := NewUserManager(database.Conn(), "secret")
	h := NewAuthHandler(mgr)

	req := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(`{"current_password":"old","new_password":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleChangePassword().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJSONHelpers(t *testing.T) {
	var statusCode int
	write := func(code int, b []byte) {
		statusCode = code
		_ = b
	}

	JSON(write, http.StatusCreated, map[string]string{"key": "value"})
	if statusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", statusCode)
	}

	JSONError(write, http.StatusBadRequest, "bad request")
	if statusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", statusCode)
	}
}
