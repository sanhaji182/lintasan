package expprovider

// credential_store.go — Encrypted Credential Store for Experimental Providers (V1).
//
// This provides dashboard-managed credential persistence for Experimental
// providers. Credentials are stored AES-256-GCM encrypted in SQLite, never
// plaintext. The encryption key is derived from the application's master_key
// via SHA-256 (compatible with future E2 vault migration).
//
// PRIORITY ORDER (CredentialSource resolution):
//   1. Dashboard credential (encrypted in DB) — highest priority
//   2. Environment variable (os.Getenv) — fallback
//   3. Missing — no credential available
//
// SECURITY:
//   - Secrets are NEVER returned in full via API (masked: first 6 + last 4 chars)
//   - Encryption at rest (AES-256-GCM, 12-byte nonce, per-value)
//   - Key derivation: SHA-256(master_key) — deterministic, no extra secret needed
//   - Credentials are NEVER logged

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// DashboardCredentialStore manages encrypted credentials for Experimental
// providers in SQLite. It implements CredentialSource with the priority chain:
// dashboard > environment > missing.
type DashboardCredentialStore struct {
	db        *sql.DB
	masterKey string // raw master key for derivation
}

// NewDashboardCredentialStore creates a credential store backed by the given DB.
// masterKey is the application's master_key used to derive the encryption key.
// The caller must ensure the credential table exists (via EnsureCredentialTable).
func NewDashboardCredentialStore(db *sql.DB, masterKey string) *DashboardCredentialStore {
	return &DashboardCredentialStore{db: db, masterKey: masterKey}
}

// EnsureCredentialTable creates the encrypted credentials table if it doesn't exist.
func EnsureCredentialTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS experimental_credentials (
			provider_name TEXT PRIMARY KEY,
			encrypted_value TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'dashboard',
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)
	`)
	return err
}

// CredentialStatus represents the status of a provider's credential.
type CredentialStatus struct {
	Provider    string  `json:"provider"`
	Configured  bool    `json:"configured"`
	Source      string  `json:"source"`       // "dashboard", "environment", "none"
	MaskedValue string  `json:"masked_value"` // e.g. "sk-abc************************xyz"
	EnvVar      string  `json:"env_var"`
	UpdatedAt   *string `json:"updated_at,omitempty"`
}

// SetCredential encrypts and stores a credential for a provider.
func (s *DashboardCredentialStore) SetCredential(ctx context.Context, providerName, secret string) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("credential cannot be empty")
	}
	encrypted, err := s.encrypt(secret)
	if err != nil {
		return fmt.Errorf("encrypt credential: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO experimental_credentials (provider_name, encrypted_value, source, created_at, updated_at)
		VALUES (?, ?, 'dashboard', ?, ?)
		ON CONFLICT(provider_name) DO UPDATE SET
			encrypted_value = excluded.encrypted_value,
			source = 'dashboard',
			updated_at = excluded.updated_at
	`, providerName, encrypted, now, now)
	return err
}

// DeleteCredential removes a stored credential for a provider.
func (s *DashboardCredentialStore) DeleteCredential(ctx context.Context, providerName string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM experimental_credentials WHERE provider_name = ?`, providerName)
	return err
}

// GetCredential retrieves and decrypts a credential. Returns ("", false) if not found.
func (s *DashboardCredentialStore) GetCredential(ctx context.Context, providerName string) (string, bool) {
	var encrypted string
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_value FROM experimental_credentials WHERE provider_name = ?`, providerName).Scan(&encrypted)
	if err != nil {
		return "", false
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return "", false
	}
	return secret, true
}

// GetStatus returns the credential status for a provider (checks dashboard then env).
func (s *DashboardCredentialStore) GetStatus(ctx context.Context, providerName, envVar string) CredentialStatus {
	status := CredentialStatus{
		Provider: providerName,
		EnvVar:   envVar,
	}

	// Check dashboard credential first
	var encrypted, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_value, updated_at FROM experimental_credentials WHERE provider_name = ?`, providerName).Scan(&encrypted, &updatedAt)
	if err == nil {
		secret, decErr := s.decrypt(encrypted)
		if decErr == nil && secret != "" {
			status.Configured = true
			status.Source = "dashboard"
			status.MaskedValue = maskSecret(secret)
			status.UpdatedAt = &updatedAt
			return status
		}
	}

	// Check environment variable
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			status.Configured = true
			status.Source = "environment"
			status.MaskedValue = maskSecret(v)
			return status
		}
	}

	// No credential available
	status.Configured = false
	status.Source = "none"
	return status
}

// Credential implements CredentialSource interface with priority:
// dashboard > environment > missing.
func (s *DashboardCredentialStore) Credential(provider string) (string, bool) {
	// Priority 1: Dashboard credential
	ctx := context.Background()
	if secret, ok := s.GetCredential(ctx, provider); ok && secret != "" {
		return secret, true
	}

	// Priority 2: Environment variable (lookup via descriptor)
	// This requires knowing the env var name for the provider.
	// The caller should use DashboardCredentialSource which wraps this with env var mapping.
	return "", false
}

// --- Encryption helpers (AES-256-GCM) ---

func (s *DashboardCredentialStore) deriveKey() []byte {
	h := sha256.Sum256([]byte(s.masterKey))
	return h[:]
}

func (s *DashboardCredentialStore) encrypt(plaintext string) (string, error) {
	key := s.deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *DashboardCredentialStore) decrypt(encoded string) (string, error) {
	key := s.deriveKey()
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- DashboardCredentialSource wraps DashboardCredentialStore with env var fallback ---

// DashboardCredentialSource implements CredentialSource with the full priority chain:
// dashboard credential > environment variable > missing.
type DashboardCredentialSource struct {
	store   *DashboardCredentialStore
	envMap  map[string]string // provider name -> env var name
}

// NewDashboardCredentialSource creates a credential source that checks dashboard
// first, then falls back to environment variables.
func NewDashboardCredentialSource(store *DashboardCredentialStore, envMap map[string]string) *DashboardCredentialSource {
	return &DashboardCredentialSource{store: store, envMap: envMap}
}

// Credential implements CredentialSource with priority: dashboard > env > missing.
func (s *DashboardCredentialSource) Credential(provider string) (string, bool) {
	// Priority 1: Dashboard credential (encrypted in DB)
	ctx := context.Background()
	if secret, ok := s.store.GetCredential(ctx, provider); ok && secret != "" {
		return secret, true
	}

	// Priority 2: Environment variable
	if envVar, ok := s.envMap[provider]; ok && envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			return v, true
		}
	}

	// Priority 3: Missing
	return "", false
}

// --- Helpers ---

// maskSecret masks a secret for display: shows first 6 and last 4 chars.
// e.g. "sk-abcdefghijklmnop" → "sk-abc**********mnop"
func maskSecret(secret string) string {
	if len(secret) <= 10 {
		return strings.Repeat("*", len(secret))
	}
	prefix := 6
	suffix := 4
	if len(secret) < prefix+suffix+4 {
		prefix = 3
		suffix = 3
	}
	masked := len(secret) - prefix - suffix
	return secret[:prefix] + strings.Repeat("*", masked) + secret[len(secret)-suffix:]
}

// compile-time assertion
var _ CredentialSource = (*DashboardCredentialSource)(nil)
