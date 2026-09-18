// SessionStore manages active Hoplite session context for thread continuity:
// maps (user_id + project_id) -> (thread_id, expires_at, last_activity).
// All stored in-memory with TTL cleanup and no persistence to disk.
package server

import (
	"hash/fnv"
	"sync"
	"time"
)

type hopliteSession struct {
	userID       string    // JWT subject or API key issuer
	projectID    string    // decoded from model ID (e.g., "proj_73567a1d915940fb853c7586c842c0f2")
	threadID     string    // active Hoplite thread
	expiresAt    time.Time // TTL expiration
	lastActivity time.Time
}

// SessionStore tracks active sessions for automatic thread continuation.
type SessionStore struct {
	sync.RWMutex
	memory map[uint64]*hopliteSession // hashed key -> session
	ttl    time.Duration              // default 30m expiry
}

// NewSessionStore creates an in-memory session tracker with optional TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	ss := &SessionStore{
		memory: make(map[uint64]*hopliteSession),
		ttl:    ttl,
	}
	// Background cleaner for expired sessions
	go ss.cleanExpired()
	return ss
}

// keyFor hashes (userID, projectID) into uint64.
func (s *SessionStore) keyFor(userID, projectID string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(userID))
	h.Write([]byte("\x00")) // separator to avoid collision
	h.Write([]byte(projectID))
	return h.Sum64()
}

// Load retrieves active session or nil if not found/expired.
func (s *SessionStore) Load(userID, projectID string) *hopliteSession {
	s.RLock()
	defer s.RUnlock()
	key := s.keyFor(userID, projectID)
	session, ok := s.memory[key]
	if !ok || session.expiresAt.Before(time.Now()) {
		return nil
	}
	return session
}

// Store saves or updates active session, clearing any prior stale entry.
func (s *SessionStore) Store(userID, projectID, threadID string) {
	s.Lock()
	defer s.Unlock()
	key := s.keyFor(userID, projectID)
	now := time.Now()
	s.memory[key] = &hopliteSession{
		userID:       userID,
		projectID:    projectID,
		threadID:     threadID,
		expiresAt:    now.Add(s.ttl),
		lastActivity: now,
	}
}

// Expire removes session by key; returns whether deleted.
func (s *SessionStore) Expire(userID, projectID string) bool {
	s.Lock()
	defer s.Unlock()
	key := s.keyFor(userID, projectID)
	if _, ok := s.memory[key]; ok {
		delete(s.memory, key)
		return true
	}
	return false
}

// cleanExpired periodically removes expired sessions.
func (s *SessionStore) cleanExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.Lock()
		for k, sess := range s.memory {
			if sess.expiresAt.Before(time.Now()) {
				delete(s.memory, k)
			}
		}
		s.Unlock()
	}
}
