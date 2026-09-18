package server

import (
	"testing"
	"time"
)

func TestHopliteSessionStoreIsolatesAndExpiresSessions(t *testing.T) {
	store := NewSessionStore(15 * time.Millisecond)
	store.Store("user-1", "account-a\x00project-a", "thr_a")
	store.Store("user-2", "account-a\x00project-a", "thr_b")

	if got := store.Load("user-1", "account-a\x00project-a"); got == nil || got.threadID != "thr_a" {
		t.Fatalf("user-1 session = %#v", got)
	}
	if got := store.Load("user-2", "account-a\x00project-a"); got == nil || got.threadID != "thr_b" {
		t.Fatalf("user-2 session = %#v", got)
	}
	time.Sleep(20 * time.Millisecond)
	if got := store.Load("user-1", "account-a\x00project-a"); got != nil {
		t.Fatalf("expired session remained: %#v", got)
	}
}
