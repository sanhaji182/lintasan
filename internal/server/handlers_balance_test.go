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

func TestParseKiloBalanceRemaining(t *testing.T) {
	body := []byte(`{"microdollars_used":2500000,"total_microdollars_acquired":10000000}`)
	info := parseKiloBalance(body)
	if info.ProviderType != "kilo" {
		t.Errorf("provider_type = %q, want kilo", info.ProviderType)
	}
	if info.Balance != "$7.50" {
		t.Errorf("balance = %q, want $7.50", info.Balance)
	}
	if !strings.Contains(info.RateInfo, "remaining") {
		t.Errorf("rate_info should mention remaining, got %q", info.RateInfo)
	}
}

func TestParseKiloBalanceZero(t *testing.T) {
	info := parseKiloBalance([]byte(`{"microdollars_used":0,"total_microdollars_acquired":0}`))
	if info.Balance != "$0.00" {
		t.Errorf("balance = %q, want $0.00", info.Balance)
	}
}
