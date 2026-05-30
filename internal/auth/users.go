package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// User represents a dashboard user.
type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"` // never serialize
	Role               string    `json:"role"` // "admin" | "user"
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UserManager manages dashboard users.
type UserManager struct {
	db     *sql.DB
	secret string // JWT signing secret
}

// NewUserManager creates a new UserManager.
func NewUserManager(db *sql.DB, secret string) *UserManager {
	return &UserManager{db: db, secret: secret}
}

// CreateUser creates a new user with hashed password.
func (m *UserManager) CreateUser(username, password, role string) (*User, error) {
	// Check uniqueness
	var count int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count); err != nil {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("username already exists")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	id := fmt.Sprintf("user_%s_%d", username, time.Now().UnixNano())
	now := time.Now().UTC()

	_, err = m.db.Exec(
		`INSERT INTO users (id, username, password_hash, role, created_at, updated_at) 
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, username, hash, role, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	return &User{
		ID:        id,
		Username:  username,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Authenticate verifies username + password and returns a JWT token.
func (m *UserManager) Authenticate(username, password string) (string, *User, error) {
	user, err := m.GetByUsername(username)
	if err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	if !VerifyPassword(password, user.PasswordHash) {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	// Generate JWT
	claims := JWTClaims{
		Sub:      user.ID,
		Username: user.Username,
		Role:     user.Role,
		Iat:      time.Now().Unix(),
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	}

	token, err := GenerateJWT(claims, m.secret)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// ValidateToken validates a JWT and returns the user.
func (m *UserManager) ValidateToken(token string) (*User, error) {
	claims, err := ValidateJWT(token, m.secret)
	if err != nil {
		return nil, err
	}

	return m.GetByID(claims.Sub)
}

// GetByUsername retrieves a user by username.
func (m *UserManager) GetByUsername(username string) (*User, error) {
	var u User
	var createdAt, updatedAt string
	var mustChange int
	err := m.db.QueryRow(
		"SELECT id, username, password_hash, role, COALESCE(must_change_password, 0), created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &mustChange, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.MustChangePassword = mustChange == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &u, nil
}

// GetByID retrieves a user by ID.
func (m *UserManager) GetByID(id string) (*User, error) {
	var u User
	var createdAt, updatedAt string
	var mustChange int
	err := m.db.QueryRow(
		"SELECT id, username, password_hash, role, COALESCE(must_change_password, 0), created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &mustChange, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	u.MustChangePassword = mustChange == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &u, nil
}

// ListUsers returns all users.
func (m *UserManager) ListUsers() ([]User, error) {
	rows, err := m.db.Query(
		"SELECT id, username, role, created_at, updated_at FROM users ORDER BY created_at",
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var createdAt, updatedAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &createdAt, &updatedAt); err != nil {
			continue
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

// SeedAdmin creates the default admin account if no users exist.
// The password is randomly generated (never hardcoded in source) and the
// account is flagged must_change_password so the operator is forced to rotate
// it on first login. The generated password is returned so the caller can
// surface it once on stderr for first-run setup.
func (m *UserManager) SeedAdmin(adminUsername string) (string, error) {
	var count int
	m.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		return "", nil // already seeded
	}

	// 24 random bytes → URL-safe password. Never persisted in plaintext.
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(buf)

	u, err := m.CreateUser(adminUsername, password, "admin")
	if err != nil {
		return "", err
	}
	// Force rotation on first login.
	if _, err := m.db.Exec("UPDATE users SET must_change_password = 1 WHERE id = ?", u.ID); err != nil {
		return "", fmt.Errorf("flag seeded admin for rotation: %w", err)
	}
	return password, nil
}

// ChangePassword updates a user's password and clears the must_change_password
// flag. It verifies the current password first.
func (m *UserManager) ChangePassword(userID, currentPassword, newPassword string) error {
	u, err := m.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if !VerifyPassword(currentPassword, u.PasswordHash) {
		return fmt.Errorf("current password incorrect")
	}
	if len(newPassword) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	if newPassword == currentPassword {
		return fmt.Errorf("new password must differ from current password")
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = m.db.Exec(
		"UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?",
		hash, time.Now().UTC().Format(time.RFC3339), userID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// AdminCount returns the number of admin users. Used by the bootstrap/active
// state machine to decide whether setup is complete.
func (m *UserManager) AdminCount() int {
	var n int
	m.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&n)
	return n
}

// LoginResponse is the JSON response for login.
type LoginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID                 string `json:"id"`
		Username           string `json:"username"`
		Role               string `json:"role"`
		MustChangePassword bool   `json:"must_change_password"`
	} `json:"user"`
}

func NewLoginResponse(token string, user *User) LoginResponse {
	resp := LoginResponse{Token: token}
	resp.User.ID = user.ID
	resp.User.Username = user.Username
	resp.User.Role = user.Role
	resp.User.MustChangePassword = user.MustChangePassword
	return resp
}

// --- JSON helpers ---

func JSON(w func(int, []byte), status int, data interface{}) {
	b, _ := json.Marshal(data)
	w(status, b)
}

func JSONError(w func(int, []byte), status int, message string) {
	b, _ := json.Marshal(map[string]string{"error": message})
	w(status, b)
}
