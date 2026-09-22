package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// TestQoderExcludedFromSDKSeam is the regression test for a precedence bug that
// produced a completely misleading failure.
//
// The Provider SDK seam is checked before the format branches and accepts every
// format except commandcode. With provider_sdk_enabled=true, a Qoder connection
// therefore reached the generic SDK provider, which built a plain
// OpenAI-compatible POST to {base_url}/chat/completions carrying the raw
// credential. Upstream answered "TOKEN_INVALID: invalid apikey" — an error that
// reads as a bad credential and conceals the real cause: the Qoder provider was
// never invoked.
//
// Two things must hold: the seam must refuse qoder, and the format branch must be
// reachable. This asserts the first directly; the second is covered by the
// live wiring probe.
func TestQoderExcludedFromSDKSeam(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, []byte(minimalQoderTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(qoder.TemplateEnvVar, path)

	h := newTestProxyHandlerWithSettings(t, map[string]string{
		"qoder_enabled":        "true",
		"provider_sdk_enabled": "true",
	})

	if !h.qoderEnabled() {
		t.Fatal("provider should be active with the flag and a template")
	}
	if h.providerSDKEligible(&Connection{Format: qoderFormat}) {
		t.Error("the SDK seam must refuse a qoder connection, or it will be sent as a generic OpenAI request")
	}
	// commandcode was already excluded; keep both exclusions pinned.
	if h.providerSDKEligible(&Connection{Format: "commandcode"}) {
		t.Error("commandcode must remain excluded from the SDK seam")
	}
	// An ordinary format must still be eligible, or the exclusion has been made
	// too broad and would disable the SDK for every real provider.
	if !h.providerSDKEligible(&Connection{Format: "openai"}) {
		t.Error("ordinary formats must still use the SDK seam")
	}
}
