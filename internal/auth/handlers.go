package auth

import (
	"encoding/json"
	"net/http"
)

// AuthHandler holds HTTP handlers for authentication endpoints.
type AuthHandler struct {
	mgr *UserManager
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(mgr *UserManager) *AuthHandler {
	return &AuthHandler{mgr: mgr}
}

// LoginRequest is the request body for POST /api/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLogin handles POST /api/auth/login.
func (h *AuthHandler) HandleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		if req.Username == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
			return
		}

		token, user, err := h.mgr.Authenticate(req.Username, req.Password)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}

		// Set cookie for dashboard SPA
		http.SetCookie(w, &http.Cookie{
			Name:     "lintasan_token",
			Value:    token,
			Path:     "/",
			HttpOnly: false, // SvelteKit needs to read it for client-side redirect
			Secure:   false, // localhost dev
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400, // 24 hours
		})

		resp := NewLoginResponse(token, user)
		writeJSON(w, http.StatusOK, resp)
	}
}

// HandleMe handles GET /api/auth/me.
func (h *AuthHandler) HandleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		user := GetUser(r)
		if user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":         user.ID,
			"username":   user.Username,
			"role":       user.Role,
			"must_change_password": user.MustChangePassword,
			"created_at": user.CreatedAt,
		})
	}
}

// HandleLogout handles POST /api/auth/logout.
func (h *AuthHandler) HandleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "lintasan_token",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
		})
		writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
	}
}

// HandleListUsers handles GET /api/auth/users (admin only).
func (h *AuthHandler) HandleListUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		user := GetUser(r)
		if user == nil || user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
			return
		}

		users, err := h.mgr.ListUsers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, users)
	}
}

// HandleCreateUser handles POST /api/auth/users (admin only).
func (h *AuthHandler) HandleCreateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		adminUser := GetUser(r)
		if adminUser == nil || adminUser.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Username == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}

		user, err := h.mgr.CreateUser(req.Username, req.Password, req.Role)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, user)
	}
}

// HandleChangePassword handles POST /api/auth/change-password.
// Authenticated user rotates their own password; clears must_change_password.
func (h *AuthHandler) HandleChangePassword() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		user := GetUser(r)
		if user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := h.mgr.ChangePassword(user.ID, req.CurrentPassword, req.NewPassword); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
