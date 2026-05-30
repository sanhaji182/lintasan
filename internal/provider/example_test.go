package provider_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// Example shows the intended end-to-end use of the Provider SDK foundation:
// register a provider, resolve it (with a safe fallback), and dispatch a
// request where the CALLER owns the HTTP transport — so reliability wraps the
// provider from the outside. None of this is wired into the live proxy; it is a
// runnable demonstration of the contract shape.
func Example() {
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewDefaultProvider("openai"))

	// Resolve by name; unknown names fall back to a generic provider so a
	// connection with no specialized provider keeps working (migration safety net).
	p := reg.Resolve("openai", provider.NewDefaultProvider("generic"))

	conn := &provider.ConnConfig{BaseURL: "https://api.openai.com", APIKey: "sk-demo"}
	req := &provider.Request{
		Model:   "gpt-4o",
		Body:    []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		Headers: http.Header{},
	}

	// The caller supplies the HTTP transport. In the real router this is where
	// circuit/retry/fallback/hedge wrap the call — the provider never sees it.
	httpDo := func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello"}}]}`)),
			Header:     http.Header{},
		}, nil
	}

	resp, err := provider.Dispatch(context.Background(), p, req, conn, httpDo, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%d body=%s\n", resp.Status, resp.Body)
	// Output: status=200 body={"choices":[{"message":{"content":"hello"}}]}
}
