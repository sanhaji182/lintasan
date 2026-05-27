// Package memory provides a pure-Go TF-IDF embedder and Redis-backed vector store.
package memory

import (
	"fmt"
	"os"
)

// MemoryStore is the top-level memory service wrapping a StoreManager and Client.
type MemoryStore struct {
	Store  *StoreManager
	client *Client
}

// Config holds Redis connection parameters.
type Config struct {
	Addr string
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
		Store:  NewStoreManager(client),
		client: client,
	}, nil
}

// NewLazy creates a MemoryStore that gracefully degrades if Redis is unavailable.
// On failure, it returns a MemoryStore with Store=nil and client=nil — callers
// must check Available() before using Store methods.
func NewLazy(cfg Config) *MemoryStore {
	if cfg.Addr == "" {
		if addr := os.Getenv("REDIS_ADDR"); addr != "" {
			cfg.Addr = addr
		} else {
			cfg.Addr = "127.0.0.1:6379"
		}
	}
	client, err := NewClient(cfg.Addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory: Redis connect failed, degrading gracefully: %v\n", err)
		return &MemoryStore{Store: nil, client: nil}
	}
	if _, err := client.Do("PING"); err != nil {
		fmt.Fprintf(os.Stderr, "memory: Redis ping failed, degrading gracefully: %v\n", err)
		client.Close()
		return &MemoryStore{Store: nil, client: nil}
	}
	return &MemoryStore{
		Store:  NewStoreManager(client),
		client: client,
	}
}

// Available returns true if Redis is connected and ready for operations.
func (ms *MemoryStore) Available() bool {
	return ms.client != nil && ms.Store != nil
}

// Close shuts down the Redis connection.
func (ms *MemoryStore) Close() error {
	if ms.client == nil {
		return nil
	}
	return ms.client.Close()
}
