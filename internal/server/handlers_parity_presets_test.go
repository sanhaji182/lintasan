package server

import "testing"

func TestProviderPresetsCommandCodeAlphaUsesDiscoveryEndpoint(t *testing.T) {
	for _, preset := range providerPresets() {
		if preset["id"] != "commandcode-alpha" {
			continue
		}
		if got := preset["modelsPath"]; got != "/provider/v1/models" {
			t.Fatalf("commandcode-alpha modelsPath = %q, want %q", got, "/provider/v1/models")
		}
		return
	}
	t.Fatal("commandcode-alpha preset not found")
}
