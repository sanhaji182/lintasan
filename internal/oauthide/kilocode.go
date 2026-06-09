package oauthide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	KilocodeInitiateURL = "https://api.kilo.ai/api/device-auth/codes"
	KilocodePollURLBase = "https://api.kilo.ai/api/device-auth/codes"
	KilocodeAPIBase     = "https://api.kilo.ai"
)

func StartKilocodeDevice() (*DeviceStart, error) {
	req, err := http.NewRequest(http.MethodPost, KilocodeInitiateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("too many pending authorization requests; try again later")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kilocode initiate HTTP %d: %s", resp.StatusCode, truncateErr(string(body)))
	}
	var data struct {
		Code            string `json:"code"`
		VerificationURL string `json:"verificationUrl"`
		ExpiresIn       int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Code == "" {
		return nil, fmt.Errorf("kilocode initiate missing code")
	}
	exp := data.ExpiresIn
	if exp <= 0 {
		exp = 300
	}
	return &DeviceStart{
		DeviceCode:              data.Code,
		UserCode:                data.Code,
		VerificationURI:         data.VerificationURL,
		VerificationURIComplete: data.VerificationURL,
		ExpiresIn:               exp,
		Interval:                3,
	}, nil
}

func PollKilocodeOnce(deviceCode string) (*DevicePollResult, error) {
	url := KilocodePollURLBase + "/" + deviceCode
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case 202:
		return &DevicePollResult{Pending: true}, nil
	case 403:
		return &DevicePollResult{Error: "authorization denied by user"}, nil
	case 410:
		return &DevicePollResult{Error: "authorization code expired"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &DevicePollResult{Error: fmt.Sprintf("poll failed HTTP %d", resp.StatusCode)}, nil
	}
	var data struct {
		Status    string `json:"status"`
		Token     string `json:"token"`
		UserEmail string `json:"userEmail"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Status != "approved" || data.Token == "" {
		return &DevicePollResult{Pending: true}, nil
	}
	orgID := ""
	profReq, _ := http.NewRequest(http.MethodGet, KilocodeAPIBase+"/api/profile", nil)
	profReq.Header.Set("Authorization", "Bearer "+data.Token)
	if pr, err := httpClient.Do(profReq); err == nil {
		defer pr.Body.Close()
		if pr.StatusCode >= 200 && pr.StatusCode < 300 {
			var profile struct {
				Organizations []struct {
					ID string `json:"id"`
				} `json:"organizations"`
			}
			if json.NewDecoder(io.LimitReader(pr.Body, 1<<20)).Decode(&profile) == nil && len(profile.Organizations) > 0 {
				orgID = profile.Organizations[0].ID
			}
		}
	}
	meta, _ := json.Marshal(map[string]any{"email": data.UserEmail, "orgId": orgID})
	return &DevicePollResult{
		Done:        true,
		AccessToken: data.Token,
		ExpiresIn:   86400,
		FlowMeta:    string(meta),
	}, nil
}