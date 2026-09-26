package provider

import "testing"

func TestRoutingPoolIdentityPrefersConfiguredPool(t *testing.T) {
	got := RoutingPoolIdentity("qoder", "https://api.qoder.com", "/chat/completions", "qoder-team")
	if got != "pool:qoder-team" {
		t.Fatalf("RoutingPoolIdentity() = %q, want pool:qoder-team", got)
	}
}

func TestRoutingPoolIdentityGroupsAccountsByProtocolEndpoint(t *testing.T) {
	a := RoutingPoolIdentity("qoder", "https://API.QODER.COM/", "/chat/completions", "")
	b := RoutingPoolIdentity("qoder", "https://api.qoder.com/v1", "/chat/completions", "")
	if a != b {
		t.Fatalf("same provider accounts got different identities: %q != %q", a, b)
	}

	alpha := RoutingPoolIdentity("commandcode", "https://api.commandcode.ai", "/alpha/generate", "")
	official := RoutingPoolIdentity("commandcode", "https://api.commandcode.ai", "/v1/chat/completions", "")
	if alpha == official {
		t.Fatalf("different provider protocols collapsed to %q", alpha)
	}
}
