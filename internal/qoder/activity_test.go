package qoder

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateCosySignature(t *testing.T) {
	fixedTime := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	secret := "cosy&war, war never changes"
	dateStr, sig := GenerateCosySignature(secret, fixedTime)

	expectedDate := "Mon, 05 Oct 2026 12:00:00 GMT"
	if dateStr != expectedDate {
		t.Fatalf("expected date %q, got %q", expectedDate, dateStr)
	}

	expectedRaw := fmt.Sprintf("%s&%s", secret, expectedDate)
	h := md5.Sum([]byte(expectedRaw))
	expectedSig := hex.EncodeToString(h[:])

	if sig != expectedSig {
		t.Fatalf("expected signature %q, got %q", expectedSig, sig)
	}
}

func TestSetCosyHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://openapi.qoder.sh/api/v2/quota/usage", nil)
	if err != nil {
		t.Fatal(err)
	}

	token := "jt-test-12345"
	machineID := "mid-device-999"
	fixedTime := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	SetCosyHeaders(req, token, machineID, fixedTime)

	if req.Header.Get("Authorization") != "Bearer "+token {
		t.Errorf("Authorization header mismatch")
	}
	if req.Header.Get("Cosy-Version") != CosyVersion {
		t.Errorf("Cosy-Version header mismatch")
	}
	if req.Header.Get("Cosy-ClientType") != CosyClientType {
		t.Errorf("Cosy-ClientType header mismatch")
	}
	if req.Header.Get("Cosy-MachineId") != machineID {
		t.Errorf("Cosy-MachineId header mismatch")
	}
	if req.Header.Get("appcode") != CosyAppCode {
		t.Errorf("appcode header mismatch")
	}
	if req.Header.Get("login-version") != "v2" {
		t.Errorf("login-version header mismatch")
	}
	if req.Header.Get("date") == "" {
		t.Errorf("date header missing")
	}
	if req.Header.Get("signature") == "" {
		t.Errorf("signature header missing")
	}
}

func TestExchangePATToJobToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobToken/exchange" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Cosy-ClientType") != "5" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["personal_token"] != "pt-valid-credential-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"token": "jt-new-generated-token-123",
			"refresh_token": "rt-refresh-token-456",
			"expires_at": 1790000000,
			"refresh_token_expires_at": 1795000000
		}`))
	}))
	defer server.Close()

	mgr := NewSessionManager("salt", "global", server.Client())

	origHost := CheckinHostGlobal
	defer func() {
		// we test directly using custom test server transport
	}()

	// Test via an http.Client with transport redirecting CheckinHostGlobal to server.URL
	customClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			targetURL := server.URL + req.URL.Path
			if req.URL.RawQuery != "" {
				targetURL += "?" + req.URL.RawQuery
			}
			newReq, _ := http.NewRequestWithContext(req.Context(), req.Method, targetURL, req.Body)
			newReq.Header = req.Header
			return server.Client().Do(newReq)
		}),
	}
	mgr.client = customClient

	res, err := mgr.ExchangePATToJobToken(context.Background(), "pt-valid-credential-key")
	if err != nil {
		t.Fatalf("ExchangePATToJobToken failed: %v (origHost=%s)", err, origHost)
	}
	if res.Token != "jt-new-generated-token-123" {
		t.Errorf("unexpected token: %s", res.Token)
	}
}

func TestActivityEligibilityAndClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify signature and date header
		if r.Header.Get("signature") == "" || r.Header.Get("date") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if r.URL.Path == "/api/v2/activity/claim/eligibility" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"code": 200,
				"message": "success",
				"data": {
					"activities": [
						{
							"activityId": "act-daily-bonus-01",
							"name": "Daily Check-in 100 Credits",
							"status": "available",
							"credits": 100
						},
						{
							"activityId": "act-view-only",
							"name": "Subscription promo",
							"status": "claimed",
							"credits": 0
						}
					]
				}
			}`))
			return
		}

		if r.URL.Path == "/api/v2/activity/claim" {
			actID := r.URL.Query().Get("activityId")
			if actID == "act-daily-bonus-01" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"code": 200,
					"message": "claimed",
					"data": { "grantId": "grant-9988", "amount": 100 }
				}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code": 400, "message": "already claimed"}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	mgr := NewSessionManager("salt", "global", nil)
	mgr.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			targetURL := server.URL + req.URL.Path
			if req.URL.RawQuery != "" {
				targetURL += "?" + req.URL.RawQuery
			}
			newReq, _ := http.NewRequestWithContext(req.Context(), req.Method, targetURL, req.Body)
			newReq.Header = req.Header
			return server.Client().Do(newReq)
		}),
	}

	// 1. Eligibility
	elig, err := mgr.CheckActivityEligibility(context.Background(), "jt-valid-token", "mid-001")
	if err != nil {
		t.Fatalf("CheckActivityEligibility failed: %v", err)
	}
	if len(elig.Activities) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(elig.Activities))
	}
	if elig.Activities[0].ActivityID != "act-daily-bonus-01" || elig.Activities[0].Credits != 100 {
		t.Errorf("unexpected activity item: %+v", elig.Activities[0])
	}

	// 2. Claim
	claimRes, err := mgr.ClaimActivity(context.Background(), "jt-valid-token", "mid-001", "act-daily-bonus-01")
	if err != nil {
		t.Fatalf("ClaimActivity failed: %v", err)
	}
	if !claimRes.Success || claimRes.Code != 200 {
		t.Errorf("claim did not succeed: %+v", claimRes)
	}

	// 3. AutoClaimActivities
	autoSummary, err := mgr.AutoClaimActivities(context.Background(), "jt-valid-token", "mid-001")
	if err != nil {
		t.Fatalf("AutoClaimActivities failed: %v", err)
	}
	if !autoSummary.Success {
		t.Fatalf("AutoClaimActivities marked unsuccessful: %s", autoSummary.Error)
	}
	if len(autoSummary.Claims) != 1 {
		t.Fatalf("expected 1 claim performed, got %d", len(autoSummary.Claims))
	}
	if autoSummary.Claims[0].ActivityID != "act-daily-bonus-01" || !autoSummary.Claims[0].Success {
		t.Errorf("unexpected auto-claim entry: %+v", autoSummary.Claims[0])
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
