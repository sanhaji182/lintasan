// Package memory provides a pure-Go TF-IDF embedder and pluggable vector store.
// Backends: Redis (primary) with automatic SQLite fallback.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backend is the interface both Redis and SQLite stores implement.
type Backend interface {
	Store(key string, embedding []float64, text string, metadata map[string]string, tags []string, score float64) error
	Search(embedding []float64, topK int) ([]Memory, error)
	SearchByKeywords(query string, topK int) ([]Memory, error)
	IndexCompletion(prompt Prompt, response string, score float64, tags []string, promptTokens, completionTokens int) (string, []float64, error)
	Stats() map[string]interface{}
	Delete(key string) error
	List(limit, offset int) ([]Memory, int, error)
	EnsureIndex() error
}

// MemoryStore is the top-level memory service wrapping a Backend.
type MemoryStore struct {
	Store   Backend
	backend string // "redis" or "sqlite"
}

// Config holds connection parameters.
type Config struct {
	Addr    string // Redis address (empty = try default)
	DataDir string // SQLite data directory (for fallback)
}

// New creates a MemoryStore connected to Redis.
func New(cfg Config) (*MemoryStore, error) {
	if cfg.Addr == "" {
		if addr := os.Getenv("REDIS_ADDR"); addr != "" {
			cfg.Addr = addr
		} else {
			cfg.Addr = "127.0.0.1:6379"
		}
	}
	client, err := NewClient(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	if _, err := client.Do("PING"); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &MemoryStore{
		Store:   NewStoreManager(client),
		backend: "redis",
	}, nil
}

// NewAuto tries Redis first, falls back to SQLite automatically.
func NewAuto(cfg Config) *MemoryStore {
	// Try Redis
	if cfg.Addr == "" {
		if addr := os.Getenv("REDIS_ADDR"); addr != "" {
			cfg.Addr = addr
		} else {
			cfg.Addr = "127.0.0.1:6379"
		}
	}

	client, err := NewClient(cfg.Addr)
	if err == nil {
		if _, pingErr := client.Do("PING"); pingErr == nil {
			fmt.Fprintf(os.Stderr, "memory: connected to Redis at %s\n", cfg.Addr)
			return &MemoryStore{
				Store:   NewStoreManager(client),
				backend: "redis",
			}
		}
		client.Close()
	}

	// Fallback to SQLite
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "data"
	}
	dbPath := filepath.Join(dataDir, "memory.db")
	fmt.Fprintf(os.Stderr, "memory: Redis unavailable, using SQLite at %s\n", dbPath)

	sqliteStore, err := NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory: SQLite init failed: %v — memory disabled\n", err)
		return &MemoryStore{Store: nil, backend: "none"}
	}

	return &MemoryStore{
		Store:   sqliteStore,
		backend: "sqlite",
	}
}

// NewLazy creates a MemoryStore that gracefully degrades if Redis is unavailable.
// Deprecated: use NewAuto which falls back to SQLite instead of disabling.
func NewLazy(cfg Config) *MemoryStore {
	return NewAuto(cfg)
}

// Available returns true if a backend is connected and ready.
func (ms *MemoryStore) Available() bool {
	return ms.Store != nil
}

// Backend returns the active backend name: "redis", "sqlite", or "none".
func (ms *MemoryStore) BackendName() string {
	return ms.backend
}

// Close shuts down the backend connection.
func (ms *MemoryStore) Close() error {
	if ms.Store == nil {
		return nil
	}
	switch s := ms.Store.(type) {
	case *SQLiteStore:
		return s.Close()
	default:
		return nil
	}
}
