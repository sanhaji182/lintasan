package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// --- Registry ---------------------------------------------------------------

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(NewDefaultProvider("openai")); err != nil {
		t.Fatalf("register: %v", err)
	}
	p, ok := r.Get("openai")
	if !ok || p.Name() != "openai" {
		t.Fatalf("expected openai provider, got %v ok=%v", p, ok)
	}
	if r.Len() != 1 {
		t.Fatalf("expected len 1, got %d", r.Len())
	}
}

func TestRegistryRejectsNil(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); !errors.Is(err, ErrNilProvider) {
		t.Fatalf("expected ErrNilProvider, got %v", err)
	}
}

func TestRegistryEmptyNameNormalizes(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(NewDefaultProvider("")); err != nil {
		t.Fatalf("empty name should normalize to 'default', got err %v", err)
	}
	if _, ok := r.Get("default"); !ok {
		t.Fatal("expected provider registered under 'default'")
	}
}

func TestRegistryRegisterReportReplaces(t *testing.T) {
	r := NewRegistry()
	replaced, err := r.RegisterReport(NewDefaultProvider("openai"))
	if err != nil || replaced {
		t.Fatalf("first register: replaced=%v err=%v", replaced, err)
	}
	replaced, err = r.RegisterReport(NewDefaultProvider("openai"))
	if err != nil || !replaced {
		t.Fatalf("second register should report replaced: replaced=%v err=%v", replaced, err)
	}
}

func TestRegistryResolveFallback(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewDefaultProvider("openai"))

	if p := r.Resolve("openai", NewDefaultProvider("fb")); p.Name() != "openai" {
		t.Fatalf("expected openai, got %q", p.Name())
	}
	if p := r.Resolve("totally-unknown", NewDefaultProvider("fb")); p.Name() != "fb" {
		t.Fatalf("expected fallback 'fb', got %q", p.Name())
	}
}

func TestRegistryMustGetPanics(t *testing.T) {
	r := NewRegistry()
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("MustGet should panic on missing provider")
		}
		if err, ok := rec.(error); !ok || !errors.Is(err, ErrNotRegistered) {
			t.Fatalf("expected ErrNotRegistered panic, got %v", rec)
		}
	}()
	r.MustGet("nope")
}

func TestRegistryNamesAndListByTrack(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewDefaultProvider("openai"))
	_ = r.Register(NewDefaultProvider("anthropic"))
	names := r.Names()
	if len(names) != 2 || names[0] != "anthropic" || names[1] != "openai" {
		t.Fatalf("expected sorted [anthropic openai], got %v", names)
	}
	official := r.ListByTrack(TrackOfficial)
	if len(official) != 2 {
		t.Fatalf("expected 2 official providers, got %v", official)
	}
	if len(r.ListByTrack(TrackExperimental)) != 0 {
		t.Fatal("expected 0 experimental providers")
	}
}

func TestPackageLevelHelpersUseDefault(t *testing.T) {
	if err := Register(NewDefaultProvider("pkg-level-openai")); err != nil {
		t.Fatalf("package-level Register: %v", err)
	}
	if _, ok := Get("pkg-level-openai"); !ok {
		t.Fatal("package-level Get did not see registered provider")
	}
	if p := Resolve("missing-xyz", NewDefaultProvider("fb")); p.Name() != "fb" {
		t.Fatalf("package-level Resolve fallback failed, got %q", p.Name())
	}
}

// --- Capabilities -----------------------------------------------------------

func TestCapabilitySetSatisfies(t *testing.T) {
	have := NewCapabilitySet(CapReasoning, CapToolCalling, CapStreaming)
	if !have.Satisfies(NewCapabilitySet(CapReasoning, CapToolCalling)) {
		t.Fatal("should satisfy a subset")
	}
	if have.Satisfies(NewCapabilitySet(CapVision)) {
		t.Fatal("should NOT satisfy a capability it lacks")
	}
	if !have.Has(CapStreaming) {
		t.Fatal("Has(CapStreaming) should be true")
	}
}

func TestCapabilitySetListSorted(t *testing.T) {
	s := NewCapabilitySet(CapVision, CapCoding, CapReasoning)
	got := s.List()
	want := []Capability{CapCoding, CapReasoning, CapVision}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("not sorted: got %v want %v", got, want)
		}
	}
}

func TestDefaultProviderCapabilitiesConservative(t *testing.T) {
	caps := NewDefaultProvider("x").Capabilities()
	if !caps.Has(CapStreaming) || !caps.Has(CapToolCalling) {
		t.Fatal("default should declare streaming + tool_calling")
	}
	if caps.Has(CapVision) {
		t.Fatal("default must not over-claim vision")
	}
}

// --- DefaultProvider --------------------------------------------------------

func TestDefaultProviderPrepareOpenAIShape(t *testing.T) {
	d := NewDefaultProvider("openai")
	conn := &ConnConfig{BaseURL: "https://api.openai.com/", APIKey: "sk-test"}
	req := &Request{Model: "gpt-4o", Body: []byte(`{"messages":[]}`), Headers: http.Header{}}

	up, err := d.Prepare(context.Background(), req, conn)
	if err != nil {
		t.Fatal(err)
	}
	if up.URL != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("bad url: %s", up.URL)
	}
	if up.Method != http.MethodPost {
		t.Fatalf("bad method: %s", up.Method)
	}
	if got := up.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("bad auth header: %q", got)
	}
	if string(up.Body) != `{"messages":[]}` {
		t.Fatalf("body should pass through unchanged, got %s", up.Body)
	}
}

func TestDefaultProviderPrepareFaithfulToLiveRouter(t *testing.T) {
	// The live router (proxy.go:981-987) and the connection-insert path
	// (handlers.go:205-206) BOTH treat an empty AuthPrefix as "Bearer ". The
	// current system has no representation for "no prefix", so the provider
	// mirrors that EXACTLY -- faithfulness, not a behavior change.
	d := NewDefaultProvider("custom")
	conn := &ConnConfig{
		BaseURL:    "https://api.example.com",
		APIKey:     "k",
		ChatPath:   "/openai/v1/chat/completions",
		AuthHeader: "X-Api-Key",
		AuthPrefix: "",
	}
	req := &Request{Body: []byte(`{}`), Headers: http.Header{}}
	up, err := d.Prepare(context.Background(), req, conn)
	if err != nil {
		t.Fatal(err)
	}
	if up.URL != "https://api.example.com/openai/v1/chat/completions" {
		t.Fatalf("override chatPath not honored: %s", up.URL)
	}
	if got := up.Header.Get("X-Api-Key"); got != "Bearer k" {
		t.Fatalf("expected live-faithful 'Bearer k' on custom header, got %q", got)
	}
}

func TestDefaultProviderPrepareNilArgs(t *testing.T) {
	d := NewDefaultProvider("x")
	if _, err := d.Prepare(context.Background(), nil, nil); !errors.Is(err, ErrPrepare) {
		t.Fatalf("expected ErrPrepare on nil args, got %v", err)
	}
}

func TestDefaultProviderTranslatePassthrough(t *testing.T) {
	d := NewDefaultProvider("x")
	raw := []byte(`{"choices":[{"message":{"content":"hi"}}]}`)
	out, err := d.Translate(context.Background(), raw, &Request{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Body) != string(raw) {
		t.Fatal("default translate should pass through unchanged")
	}
}

// --- Dispatch ---------------------------------------------------------------

func TestDispatchEndToEnd(t *testing.T) {
	d := NewDefaultProvider("openai")
	conn := &ConnConfig{BaseURL: "https://api.openai.com", APIKey: "k"}
	req := &Request{Model: "gpt-4o", Body: []byte(`{"messages":[]}`), Headers: http.Header{}}

	var calledWithAuth string
	fakeDo := func(r *http.Request) (*http.Response, error) {
		calledWithAuth = r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[]}`)),
			Header:     http.Header{},
		}, nil
	}
	readAll := func(resp *http.Response) ([]byte, error) { return io.ReadAll(resp.Body) }

	out, err := Dispatch(context.Background(), d, req, conn, fakeDo, readAll)
	if err != nil {
		t.Fatal(err)
	}
	if calledWithAuth != "Bearer k" {
		t.Fatalf("auth header not set by Prepare before the call: %q", calledWithAuth)
	}
	if out.Status != 200 || !strings.Contains(string(out.Body), "choices") {
		t.Fatalf("bad dispatch result: %+v", out)
	}
}

func TestDispatchDefaultReadAll(t *testing.T) {
	d := NewDefaultProvider("openai")
	conn := &ConnConfig{BaseURL: "https://x", APIKey: "k"}
	req := &Request{Body: []byte(`{}`), Headers: http.Header{}}
	fakeDo := func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader(`ok`)),
			Header:     http.Header{},
		}, nil
	}
	out, err := Dispatch(context.Background(), d, req, conn, fakeDo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != 201 || string(out.Body) != "ok" {
		t.Fatalf("default readAll path failed: %+v", out)
	}
}

func TestDispatchPropagatesPrepareError(t *testing.T) {
	d := NewDefaultProvider("x")
	called := false
	fakeDo := func(r *http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}
	_, err := Dispatch(context.Background(), d, &Request{Body: []byte(`{}`)}, nil, fakeDo, nil)
	if !errors.Is(err, ErrPrepare) {
		t.Fatalf("expected ErrPrepare, got %v", err)
	}
	if called {
		t.Fatal("httpDo must not be called when Prepare fails")
	}
}

// --- Track / contract stability --------------------------------------------

func TestDefaultProviderIsOfficial(t *testing.T) {
	if NewDefaultProvider("x").Track() != TrackOfficial {
		t.Fatal("default provider must be on the official track")
	}
}

type optionalCapsProvider struct{ *DefaultProvider }

func (o optionalCapsProvider) Embed(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error) {
	return &UpstreamRequest{URL: conn.BaseURL + "/v1/embeddings", Method: http.MethodPost, Body: req.Body}, nil
}

// plainProvider implements ONLY the core Provider interface and deliberately
// does NOT implement any optional interface (no Embed). It is the negative case
// for the type-assertion mechanism: as of F2.5 the real DefaultProvider DOES
// implement Embedder (embedder.go), so a fresh minimal stand-in is needed to
// prove the assertion correctly distinguishes non-implementers.
type plainProvider struct{}

func (plainProvider) Name() string                { return "plain" }
func (plainProvider) Track() Track                { return TrackOfficial }
func (plainProvider) Capabilities() CapabilitySet { return NewCapabilitySet() }
func (plainProvider) Prepare(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error) {
	return &UpstreamRequest{}, nil
}
func (plainProvider) Translate(ctx context.Context, raw []byte, req *Request) (*Response, error) {
	return &Response{Status: http.StatusOK, Body: raw}, nil
}

func TestOptionalInterfaceTypeAssertion(t *testing.T) {
	var p Provider = optionalCapsProvider{NewDefaultProvider("emb")}
	if _, ok := Provider(plainProvider{}).(Embedder); ok {
		t.Fatal("plainProvider (no Embed method) should not implement Embedder")
	}
	if _, ok := p.(Embedder); !ok {
		t.Fatal("optionalCapsProvider should implement Embedder")
	}
	// F2.5 contract: the real DefaultProvider now implements Embedder, so the
	// optional-interface assertion must succeed for it too.
	if _, ok := Provider(NewDefaultProvider("d")).(Embedder); !ok {
		t.Fatal("DefaultProvider must implement Embedder as of F2.5")
	}
}
