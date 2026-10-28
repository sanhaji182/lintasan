package discover

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFreeProviderModels_FiltersFreeIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"vendor/paid-model"},
			{"id":"vendor/good-model:free"},
			{"id":"vendor/another-free"},
			{"id":"vendor/free-model:free"}
		]}`))
	}))
	defer server.Close()

	p := FreeProvider{
		Name:       t.Name(),
		BaseURL:    server.URL,
		ModelsPath: "/models",
		FreeOnly:   true,
	}
	models, err := FetchFreeProviderModels(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"vendor/another-free", "vendor/free-model:free", "vendor/good-model:free"}
	if len(models) != len(want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("models = %v, want %v", models, want)
		}
	}
}

func TestFetchFreeProviderModels_ParsesArrayShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"openai"},{"id":"openai-fast"}]`))
	}))
	defer server.Close()

	models, err := FetchFreeProviderModels(context.Background(), FreeProvider{
		Name:       t.Name(),
		BaseURL:    server.URL,
		ModelsPath: "/models",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "openai" || models[1] != "openai-fast" {
		t.Fatalf("models = %v", models)
	}
}

func TestFetchFreeProviderModels_ReportsAuthStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	_, err := FetchFreeProviderModels(context.Background(), FreeProvider{
		Name:       t.Name(),
		BaseURL:    server.URL,
		ModelsPath: "/models",
	})
	var perr *ProviderModelsError
	if !errors.As(err, &perr) || perr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want ProviderModelsError with status 401", err)
	}
}

func TestFetchFreeProviderModels_NoModelsEndpoint(t *testing.T) {
	_, err := FetchFreeProviderModels(context.Background(), FreeProvider{
		Name:         t.Name(),
		NoModelFetch: true,
	})
	if err == nil {
		t.Fatal("expected an error for no_model_fetch provider")
	}
}
