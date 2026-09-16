package discover

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type readErrorRoundTripFunc func(*http.Request) (*http.Response, error)

func (f readErrorRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingReadCloser) Close() error             { return nil }

func TestFetchModelsFromProvider_ReadErrorUsesOnlyVerifiedFallbacks(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		baseURL      string
		wantFallback bool
	}{
		{name: "OpenAI", format: "openai", baseURL: "https://api.openai.com", wantFallback: true},
		{name: "Anthropic", format: "anthropic", baseURL: "https://api.anthropic.com", wantFallback: true},
		{name: "CommandCode", format: "commandcode", baseURL: "https://api.commandcode.ai", wantFallback: true},
		{name: "unknown OpenAI-compatible provider", format: "openai", baseURL: "https://unknown.example", wantFallback: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDiscoverer(nil)
			d.httpClient = &http.Client{Transport: readErrorRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       failingReadCloser{},
					Header:     make(http.Header),
				}, nil
			})}

			models, err := d.fetchModelsFromProvider(map[string]any{
				"base_url": tt.baseURL,
				"format":   tt.format,
			})
			if tt.wantFallback {
				if err != nil {
					t.Fatalf("expected verified fallback, got error: %v", err)
				}
				if len(models) == 0 {
					t.Fatal("expected verified fallback models")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "read models response") {
				t.Fatalf("expected read error for unknown provider, got models=%v err=%v", models, err)
			}
			if len(models) != 0 {
				t.Fatalf("unknown provider must fail closed, got %v", models)
			}
		})
	}
}

var _ io.ReadCloser = failingReadCloser{}
