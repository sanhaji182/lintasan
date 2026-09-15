package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type hopliteAccount struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	CredentialName   string   `json:"-"`
	IsActive         int      `json:"is_active"`
	HealthStatus     string   `json:"health_status"`
	LastTestedAt     *string  `json:"last_tested_at,omitempty"`
	LastError        string   `json:"last_error,omitempty"`
	CreditsRemaining *float64 `json:"credits_remaining,omitempty"`
	ExpiresAt        string   `json:"expires_at,omitempty"`
	CredentialSet    bool     `json:"credential_configured"`
	CredentialMasked string   `json:"credential_masked,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
	UpdatedAt        string   `json:"updated_at,omitempty"`
}

func (s *Server) ensureLegacyHopliteAccount(ctx context.Context) error {
	status := s.credStore().GetStatus(ctx, hopliteCredentialName, hopliteCredentialEnv)
	if !status.Configured {
		return nil
	}
	_, err := s.db.Conn().ExecContext(ctx, `INSERT OR IGNORE INTO hoplite_accounts(id,name,credential_name,is_active) VALUES(?,?,?,1)`, hopliteConnectionID, "Hoplite", hopliteCredentialName)
	return err
}

func (s *Server) hopliteAccounts(ctx context.Context) ([]hopliteAccount, error) {
	if err := s.ensureLegacyHopliteAccount(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.Conn().QueryContext(ctx, `SELECT id,name,credential_name,is_active,health_status,last_tested_at,last_error,credits_remaining,expires_at,created_at,updated_at FROM hoplite_accounts ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []hopliteAccount{}
	for rows.Next() {
		var a hopliteAccount
		var tested sql.NullString
		var credits sql.NullFloat64
		if err := rows.Scan(&a.ID, &a.Name, &a.CredentialName, &a.IsActive, &a.HealthStatus, &tested, &a.LastError, &credits, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if tested.Valid {
			a.LastTestedAt = &tested.String
		}
		if credits.Valid {
			a.CreditsRemaining = &credits.Float64
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		env := ""
		if out[i].ID == hopliteConnectionID {
			env = hopliteCredentialEnv
		}
		status := s.credStore().GetStatus(ctx, out[i].CredentialName, env)
		out[i].CredentialSet, out[i].CredentialMasked = status.Configured, status.MaskedValue
	}
	return out, nil
}

func (s *Server) hopliteAccountByID(ctx context.Context, id string) (hopliteAccount, bool) {
	if strings.TrimSpace(id) == "" {
		id = hopliteConnectionID
	}
	_ = s.ensureLegacyHopliteAccount(ctx)
	var a hopliteAccount
	var tested sql.NullString
	var credits sql.NullFloat64
	err := s.db.Conn().QueryRowContext(ctx, `SELECT id,name,credential_name,is_active,health_status,last_tested_at,last_error,credits_remaining,expires_at,created_at,updated_at FROM hoplite_accounts WHERE id=?`, id).Scan(&a.ID, &a.Name, &a.CredentialName, &a.IsActive, &a.HealthStatus, &tested, &a.LastError, &credits, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return hopliteAccount{}, false
	}
	if tested.Valid {
		a.LastTestedAt = &tested.String
	}
	if credits.Valid {
		a.CreditsRemaining = &credits.Float64
	}
	return a, true
}

func (s *Server) hopliteCredentialForAccount(ctx context.Context, id string) (string, bool) {
	if strings.TrimSpace(id) == "" {
		id = hopliteConnectionID
	}
	a, ok := s.hopliteAccountByID(ctx, id)
	credentialName := hopliteCredentialName
	if ok {
		credentialName = a.CredentialName
	} else if id != hopliteConnectionID {
		return "", false
	}
	if key, found := s.credStore().GetCredential(ctx, credentialName); found && strings.TrimSpace(key) != "" {
		return strings.TrimSpace(key), true
	}
	if id == hopliteConnectionID {
		if key := strings.TrimSpace(os.Getenv(hopliteCredentialEnv)); key != "" {
			return key, true
		}
	}
	return "", false
}

func (s *Server) handleHopliteAccounts(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	accounts, err := s.hopliteAccounts(r.Context())
	if err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "failed to list Hoplite accounts"})
		return
	}
	writeData(w, accounts)
}

func invalidMaskedSecret(v string) bool {
	return strings.Contains(v, "*") || strings.Contains(v, "...") || strings.Contains(v, "•")
}

func (s *Server) handleHopliteAccountCreate(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	var in struct {
		Name       string `json:"name"`
		Credential string `json:"credential"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "invalid JSON body"})
		return
	}
	in.Name, in.Credential = strings.TrimSpace(in.Name), strings.TrimSpace(in.Credential)
	if in.Name == "" || in.Credential == "" {
		writeJSONStatus(w, 400, map[string]any{"error": "name and credential are required"})
		return
	}
	if invalidMaskedSecret(in.Credential) {
		writeJSONStatus(w, 400, map[string]any{"error": "masked credential placeholders cannot be saved"})
		return
	}
	suffix := uuid.NewString()
	id := hopliteConnectionID + "-" + suffix
	credName := "hoplite-account:" + suffix
	if err := s.credStore().SetCredential(r.Context(), credName, in.Credential); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "failed to store credential"})
		return
	}
	if _, err := s.db.Conn().ExecContext(r.Context(), `INSERT INTO hoplite_accounts(id,name,credential_name,is_active) VALUES(?,?,?,1)`, id, in.Name, credName); err != nil {
		_ = s.credStore().DeleteCredential(r.Context(), credName)
		writeJSONStatus(w, 500, map[string]any{"error": "failed to create account"})
		return
	}
	a, _ := s.hopliteAccountByID(r.Context(), id)
	status := s.credStore().GetStatus(r.Context(), credName, "")
	a.CredentialSet, a.CredentialMasked = status.Configured, status.MaskedValue
	writeJSONStatus(w, http.StatusCreated, map[string]any{"data": a})
}

func (s *Server) handleHopliteAccountPatch(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	a, ok := s.hopliteAccountByID(r.Context(), id)
	if !ok {
		writeJSONStatus(w, 404, map[string]any{"error": "Hoplite account not found"})
		return
	}
	var raw map[string]any
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&raw) != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "invalid JSON body"})
		return
	}
	for field := range raw {
		if field != "name" && field != "credential" && field != "is_active" {
			writeJSONStatus(w, 400, map[string]any{"error": "unknown field: " + field})
			return
		}
	}
	credential, credentialExists := raw["credential"].(string)
	if _, exists := raw["credential"]; exists && !credentialExists {
		writeJSONStatus(w, 400, map[string]any{"error": "credential must be a string"})
		return
	}
	credential = strings.TrimSpace(credential)
	if credential != "" && invalidMaskedSecret(credential) {
		writeJSONStatus(w, 400, map[string]any{"error": "masked credential placeholders cannot be saved"})
		return
	}
	name := a.Name
	if v, exists := raw["name"]; exists {
		value, valid := v.(string)
		if !valid {
			writeJSONStatus(w, 400, map[string]any{"error": "name must be a string"})
			return
		}
		name = strings.TrimSpace(value)
		if name == "" {
			writeJSONStatus(w, 400, map[string]any{"error": "name cannot be empty"})
			return
		}
	}
	active := a.IsActive
	if v, exists := raw["is_active"]; exists {
		switch x := v.(type) {
		case bool:
			if x {
				active = 1
			} else {
				active = 0
			}
		case float64:
			if x == 1 {
				active = 1
			} else if x == 0 {
				active = 0
			} else {
				writeJSONStatus(w, 400, map[string]any{"error": "is_active must be a boolean or 0/1"})
				return
			}
		default:
			writeJSONStatus(w, 400, map[string]any{"error": "is_active must be a boolean or 0/1"})
			return
		}
	}
	if credential != "" {
		if err := s.credStore().SetCredential(r.Context(), a.CredentialName, credential); err != nil {
			writeJSONStatus(w, 500, map[string]any{"error": "failed to store credential"})
			return
		}
	}
	_, err := s.db.Conn().ExecContext(r.Context(), `UPDATE hoplite_accounts SET name=?,is_active=?,health_status=CASE WHEN ?<>'' THEN 'unknown' ELSE health_status END,last_error=CASE WHEN ?<>'' THEN '' ELSE last_error END,updated_at=? WHERE id=?`, name, active, credential, credential, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "failed to update account"})
		return
	}
	updated, _ := s.hopliteAccountByID(r.Context(), id)
	st := s.credStore().GetStatus(r.Context(), updated.CredentialName, "")
	updated.CredentialSet, updated.CredentialMasked = st.Configured, st.MaskedValue
	writeData(w, updated)
}

func (s *Server) handleHopliteAccountDelete(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	a, ok := s.hopliteAccountByID(r.Context(), id)
	if !ok {
		writeJSONStatus(w, 404, map[string]any{"error": "Hoplite account not found"})
		return
	}
	tx, err := s.db.Conn().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "failed to delete account"})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM hoplite_accounts WHERE id=?`, id); err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM experimental_credentials WHERE provider_name=?`, a.CredentialName)
	}
	if err != nil {
		_ = tx.Rollback()
		writeJSONStatus(w, 500, map[string]any{"error": "failed to delete account"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "failed to delete account"})
		return
	}
	writeData(w, map[string]any{"id": id, "status": "deleted"})
}

func encodeHoplitePart(v string) string { return base64.RawURLEncoding.EncodeToString([]byte(v)) }
func decodeHoplitePart(v string) (string, bool) {
	b, e := base64.RawURLEncoding.DecodeString(v)
	return string(b), e == nil && len(b) > 0
}
