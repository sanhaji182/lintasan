package server

import (
	"testing"
)

func TestClassifyModelTestStatus(t *testing.T) {
	cases := []struct {
		name string
		code int
		body string
		want string
	}{
		{"ok 200", 200, `{"id":"x"}`, "ok"},
		{"ok 201", 201, "", "ok"},
		{"auth 401", 401, `{"error":"invalid key"}`, "auth_error"},
		{"auth 403", 403, `{"error":"forbidden"}`, "auth_error"},
		{"not found 404", 404, `{"error":"model not found"}`, "model_not_found"},
		{"rate 429", 429, `{"error":"rate limit"}`, "rate_limited"},
		{"400 model not found body", 400, `{"error":{"message":"Model not found"}}`, "model_not_found"},
		{"400 does not exist", 400, `The model 'x' does not exist`, "model_not_found"},
		{"400 unknown model", 400, `{"error":"unknown model: xyz"}`, "model_not_found"},
		{"400 generic", 400, `{"error":"bad context length"}`, "bad_request"},
		{"500 upstream", 500, `oops`, "upstream_error"},
		{"503 upstream", 503, `unavailable`, "upstream_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyModelTestStatus(c.code, c.body)
			if got != c.want {
				t.Errorf("classifyModelTestStatus(%d, %q) = %q, want %q", c.code, c.body, got, c.want)
			}
		})
	}
}