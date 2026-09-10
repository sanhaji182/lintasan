package server

import (
	"strings"
	"testing"
)

func TestParseOpenRouterBalanceFreeTierLimit(t *testing.T) {
	body := []byte(`{"data":{"label":"sk-or-v1-xxx","limit":2,"limit_remaining":2,"usage":0.16057575,"is_free_tier":true}}`)
	info := parseOpenRouterBalance(body)
	if info.ProviderType != "openrouter" {
		t.Errorf("provider_type = %q, want openrouter", info.ProviderType)
	}
	if info.PlanType != "free_tier" {
		t.Errorf("plan_type = %q, want free_tier", info.PlanType)
	}
	if !strings.Contains(info.RateInfo, "remaining") {
		t.Errorf("rate_info should mention remaining, got %q", info.RateInfo)
	}
	if info.Balance != "" {
		t.Errorf("balance should be empty (rate badge path), got %q", info.Balance)
	}
}

func TestParseOpenRouterBalanceUnlimited(t *testing.T) {
	body := []byte(`{"data":{"limit":0,"limit_remaining":0,"usage":0.5,"is_free_tier":false}}`)
	info := parseOpenRouterBalance(body)
	if info.PlanType != "api" {
		t.Errorf("plan_type = %q, want api", info.PlanType)
	}
	if !strings.Contains(info.RateInfo, "Unlimited") {
		t.Errorf("rate_info should say Unlimited, got %q", info.RateInfo)
	}
}

func TestParseOpenRouterBalanceMalformed(t *testing.T) {
	info := parseOpenRouterBalance([]byte(`not json`))
	if info.Error == "" {
		t.Errorf("expected parse error")
	}
}
