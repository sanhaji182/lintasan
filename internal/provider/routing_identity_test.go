package provider

import "testing"

func TestRoutingPoolIdentityPrefersConfiguredPool(t *testing.T) {
	got := RoutingPoolIdentity("qoder", "https://api.qoder.com", "/chat/completions", "qoder-team")
	if got != "pool:qoder-team" {
		t.Fatalf("RoutingPoolIdentity() = %q, want pool:qoder-team", got)
	}
}

func TestRoutingPoolIdentityGroupsAccountsByProtocolEndpoint(t *testing.T) {
	a := RoutingPoolIdentity("qoder", "https://API.QODER.COM/", "/v1/chat/completions", "")
	b := RoutingPoolIdentity("qoder", "https://api.qoder.com/v1", "/v1/chat/completions", "")
	if a != b {
		t.Fatalf("same provider accounts got different identities: %q != %q", a, b)
	}

	alpha := RoutingPoolIdentity("commandcode", "https://api.commandcode.ai", "/alpha/generate", "")
	official := RoutingPoolIdentity("commandcode", "https://api.commandcode.ai", "/v1/chat/completions", "")
	if alpha == official {
		t.Fatalf("different provider protocols collapsed to %q", alpha)
	}
}

func TestRoutingPoolIdentityUsesEffectiveEndpointSchemeAndPort(t *testing.T) {
	equivalentA := RoutingPoolIdentity("openai", "https://api.example.com", "/v1/chat/completions", "")
	equivalentB := RoutingPoolIdentity("openai", "https://api.example.com/v1/", "/v1/chat/completions", "")
	if equivalentA != equivalentB {
		t.Fatalf("equivalent effective endpoints split: %q != %q", equivalentA, equivalentB)
	}

	for name, distinct := range map[string]string{
		"base path": RoutingPoolIdentity("openai", "https://api.example.com", "/chat/completions", ""),
		"scheme":    RoutingPoolIdentity("openai", "http://api.example.com", "/v1/chat/completions", ""),
		"port":      RoutingPoolIdentity("openai", "https://api.example.com:8443", "/v1/chat/completions", ""),
	} {
		if equivalentA == distinct {
			t.Fatalf("%s collapsed distinct endpoint into %q", name, equivalentA)
		}
	}
}
