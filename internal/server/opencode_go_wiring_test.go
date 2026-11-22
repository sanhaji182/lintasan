package server

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func TestOpenCodeGoSessionIDShape(t *testing.T) {
	id := OpenCodeGoSessionID("", "")
	if !strings.HasPrefix(id, "ses_") || len(id) != 36 {
		t.Fatalf("bad session shape: %q (len %d)", id, len(id))
	}
	if _, err := hex.DecodeString(id[4:]); err != nil {
		t.Errorf("session payload not hex: %q", id)
	}
	if again := OpenCodeGoSessionID("", ""); again != id {
		t.Errorf("session not deterministic: %q != %q", id, again)
	}
	native := "ses_0123456789abcdef0123456789abcdef"
	if got := OpenCodeGoSessionID(native, ""); got != native {
		t.Errorf("native session not preserved: %q", got)
	}
}

func TestOpenCodeGoHeadersApplied(t *testing.T) {
	h := http.Header{}
	applyOpenCodeGoHeaders(h, "curl/8", "", "cli")
	for _, key := range []string{"User-Agent", "x-opencode-session", "x-opencode-client", "x-opencode-project"} {
		if h.Get(key) == "" {
			t.Errorf("missing header %s", key)
		}
	}
	if h.Get("x-opencode-session") != OpenCodeGoSessionID("", "cli") {
		t.Errorf("session derived with wrong scope: %q", h.Get("x-opencode-session"))
	}
	if h.Get("x-opencode-client") != "cli" {
		t.Errorf("client tool not preserved: %q", h.Get("x-opencode-client"))
	}
	if !strings.HasPrefix(h.Get("User-Agent"), "opencode/") {
		t.Errorf("UA not pinned: %q", h.Get("User-Agent"))
	}
}
