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
	if !info.CreditKnown || info.RemainingNum != 2 {
		t.Errorf("credit metadata = known:%v remaining:%v, want known:true remaining:2", info.CreditKnown, info.RemainingNum)
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
	if info.CreditKnown {
		t.Error("unlimited OpenRouter keys must not be treated as a known credit balance")
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
	if !info.CreditKnown || info.RemainingNum != 7.5 {
		t.Errorf("credit metadata = known:%v remaining:%v, want known:true remaining:7.5", info.CreditKnown, info.RemainingNum)
	}
}

func TestParseKiloBalanceExpiry(t *testing.T) {
	body := []byte(`{"microdollars_used":2500000,"total_microdollars_acquired":10000000,"next_credit_expiration_at":"2099-11-11T10:11:58.000Z"}`)
	info := parseKiloBalance(body)
	if info.ExpiresAt != "2099-11-11T10:11:58Z" {
		t.Errorf("expires_at = %q, want 2099-11-11T10:11:58Z", info.ExpiresAt)
	}
	if info.DaysRemaining <= 0 {
		t.Errorf("days_remaining = %d, want > 0", info.DaysRemaining)
	}
	if info.BillingReset == "" {
		t.Error("billing_reset should remain populated for the existing UI")
	}
}

func TestParseKiloBalanceZero(t *testing.T) {
	info := parseKiloBalance([]byte(`{"microdollars_used":0,"total_microdollars_acquired":0}`))
	if info.Balance != "$0.00" {
		t.Errorf("balance = %q, want $0.00", info.Balance)
	}
}

func TestCommandCodePlanCaps(t *testing.T) {
	if commandCodePlanCaps["individual-go"] != 10.0 {
		t.Errorf("individual-go cap = %v, want 10.0", commandCodePlanCaps["individual-go"])
	}
	if commandCodePlanCaps["individual-goat"] != 70.0 {
		t.Errorf("individual-goat cap = %v, want 70.0", commandCodePlanCaps["individual-goat"])
	}
	if commandCodePlanCaps["individual-pro"] != 30.0 {
		t.Errorf("individual-pro cap = %v, want 30.0", commandCodePlanCaps["individual-pro"])
	}
	if commandCodePlanNames["individual-go"] != "Go" {
		t.Errorf("individual-go name = %q, want Go", commandCodePlanNames["individual-go"])
	}
}
