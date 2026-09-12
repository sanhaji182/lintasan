package server

// handlers_credentials.go — Credential Management V1 API endpoints.
//
// Admin-only endpoints for managing Experimental provider and Cloud Agent
// credentials from the dashboard. All endpoints are behind authMiddleware
// (fail-closed). Secrets are NEVER returned in full — only masked values.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/expprovider"
)

type credentialDescriptor struct {
	Name   string
	EnvVar string
}

func credentialDescriptors() []credentialDescriptor {
	descriptors := expprovider.CohortADescriptors()
	out := make([]credentialDescriptor, 0, len(descriptors)+1)
	for _, descriptor := range descriptors {
		out = append(out, credentialDescriptor{Name: descriptor.Name, EnvVar: descriptor.AuthEnvVar})
	}
	return append(out, credentialDescriptor{Name: "hoplite", EnvVar: "HOPLITE_API_KEY"})
}

func findCredentialDescriptor(name string) *credentialDescriptor {
	for _, descriptor := range credentialDescriptors() {
		if descriptor.Name == name {
			return &descriptor
		}
	}
	return nil
}

// registerCredentialRoutes wires the credential management API endpoints.
func (s *Server) registerCredentialRoutes() {
	s.mux.HandleFunc("GET /api/experimental/credentials", s.handleCredentialList)
	s.mux.HandleFunc("GET /api/experimental/credentials/{name}", s.handleCredentialStatus)
	s.mux.HandleFunc("PUT /api/experimental/credentials/{name}", s.handleCredentialSet)
	s.mux.HandleFunc("DELETE /api/experimental/credentials/{name}", s.handleCredentialDelete)
}

// credStore returns the credential store (lazily uses the master key from DB).
func (s *Server) credStore() *expprovider.DashboardCredentialStore {
	masterKey, _ := s.db.GetSetting("master_key")
	if strings.TrimSpace(masterKey) == "" && s.cfg != nil {
		masterKey = s.cfg.MasterKey
	}
	return expprovider.NewDashboardCredentialStore(s.db.Conn(), masterKey)
}

// requireCredentialManager rejects authenticated dashboard users that are not
// admins. A nil user is still authenticated by the global middleware via a
// master key or dashboard API key, preserving automation compatibility.
func requireCredentialManager(w http.ResponseWriter, r *http.Request) bool {
	if user := auth.GetUser(r); user != nil && user.Role != "admin" {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "admin access required"})
		return false
	}
	return true
}

func (s *Server) handleCredentialList(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	store := s.credStore()
	ctx := r.Context()
	statuses := make([]expprovider.CredentialStatus, 0, len(credentialDescriptors()))
	for _, descriptor := range credentialDescriptors() {
		statuses = append(statuses, store.GetStatus(ctx, descriptor.Name, descriptor.EnvVar))
	}
	writeData(w, statuses)
}

func (s *Server) handleCredentialStatus(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}
	descriptor := findCredentialDescriptor(name)
	if descriptor == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "credential target not found"})
		return
	}
	writeData(w, s.credStore().GetStatus(r.Context(), descriptor.Name, descriptor.EnvVar))
}

func (s *Server) handleCredentialSet(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}
	descriptor := findCredentialDescriptor(name)
	if descriptor == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "credential target not found"})
		return
	}
	var body struct {
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	body.Credential = strings.TrimSpace(body.Credential)
	if body.Credential == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "credential cannot be empty"})
		return
	}
	if err := s.credStore().SetCredential(r.Context(), name, body.Credential); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "failed to store credential"})
		return
	}
	writeData(w, map[string]any{
		"provider": name,
		"status":   s.credStore().GetStatus(r.Context(), descriptor.Name, descriptor.EnvVar),
		"message":  "credential stored successfully",
	})
}

func (s *Server) handleCredentialDelete(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}
	descriptor := findCredentialDescriptor(name)
	if descriptor == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "credential target not found"})
		return
	}
	if err := s.credStore().DeleteCredential(r.Context(), name); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete credential"})
		return
	}
	writeData(w, map[string]any{
		"provider": name,
		"status":   s.credStore().GetStatus(r.Context(), descriptor.Name, descriptor.EnvVar),
		"message":  "credential removed",
	})
}
