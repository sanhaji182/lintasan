package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

// ─────────────────────────────────────────────────────────────────────────────
// Non-chat proxy routing (embeddings / images / audio).
//
// Two defects these tests pin, both of which made the non-chat endpoints
// unusable in a multi-connection install:
//
//  1. NO PER-MODEL ROUTING. proxyPath called getFirstConnection(), so every
//     image/audio request went to the highest-priority active connection
//     regardless of which model was asked for. With more than one connection
//     configured — the normal case — the request landed on the wrong provider.
//
//  2. DOUBLED VERSION SEGMENT. The upstream URL was BaseURL + "/v1/audio/speech".
//     Every catalogue preset stores a base that already ends in "/v1", so the
//     real request went to "…/v1/v1/audio/speech" and 404'd.
//
// The tests drive the live handlers through the real middleware chain against
// mock upstreams, so they fail against the pre-fix code and pass after.
// ─────────────────────────────────────────────────────────────────────────────

// recordingUpstream captures the path of every request it receives and answers
// with a fixed JSON body, so a test can assert both WHICH upstream was chosen
// and WHAT path it was asked for.
type recordingUpstream struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
	auth  []string
}

func newRecordingUpstream(t *testing.T, response string) *recordingUpstream {
	t.Helper()
	ru := &recordingUpstream{}
	ru.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ru.mu.Lock()
		ru.paths = append(ru.paths, r.URL.Path)
		ru.auth = append(ru.auth, r.Header.Get("Authorization"))
		ru.mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(ru.Close)
	return ru
}

func (ru *recordingUpstream) hits() []string {
	ru.mu.Lock()
	defer ru.mu.Unlock()
	out := make([]string, len(ru.paths))
	copy(out, ru.paths)
	return out
}

// seedConn inserts a connection. baseSuffix lets a test choose the base URL
// shape ("" for a bare host, "/v1" for the versioned shape every preset uses).
func seedConn(t *testing.T, s *Server, id, baseURL, baseSuffix string, priority int) {
	t.Helper()
	if _, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, is_active, priority)
		VALUES (?, ?, ?, ?, 'openai', '/v1/chat/completions', 1, ?)`,
		id, id, baseURL+baseSuffix, "sk-"+id, priority,
	); err != nil {
		t.Fatalf("seed connection %s: %v", id, err)
	}
}

func seedModel(t *testing.T, s *Server, connID, modelID string) {
	t.Helper()
	if _, err := s.db.Conn().Exec(`
		INSERT INTO discovered_models (id, connection_id, model_id, model_name, is_active)
		VALUES (?, ?, ?, ?, 1)`,
		"dm-"+connID+"-"+modelID, connID, modelID, modelID,
	); err != nil {
		t.Fatalf("seed model %s: %v", modelID, err)
	}
}

const nonChatMasterKey = "test-master-key-for-nonchat-routing-1234567890"

func nonChatPost(t *testing.T, ts *httptest.Server, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+nonChatMasterKey)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// The core routing defect: with two connections active, the model named in the
// request body must decide which upstream is used. The decoy has the HIGHER
// priority, so a getFirstConnection-based implementation picks it and fails.
func TestNonChatRoutesByModelNotFirstConnection(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		model    string
		body     string
		wantPath string
	}{
		{
			name:     "images",
			endpoint: "/v1/images/generations",
			model:    "img-model-x",
			body:     `{"model":"img-model-x","prompt":"a cat"}`,
			wantPath: "/v1/images/generations",
		},
		{
			name:     "audio speech",
			endpoint: "/v1/audio/speech",
			model:    "tts-model-x",
			body:     `{"model":"tts-model-x","input":"hello","voice":"alloy"}`,
			wantPath: "/v1/audio/speech",
		},
		{
			name:     "embeddings",
			endpoint: "/v1/embeddings",
			model:    "emb-model-x",
			body:     `{"model":"emb-model-x","input":"hello"}`,
			wantPath: "/v1/embeddings",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, ts := newTestServer(t, &config.Config{MasterKey: nonChatMasterKey})

			decoy := newRecordingUpstream(t, `{"decoy":true}`)
			target := newRecordingUpstream(t, `{"ok":true}`)

			// The decoy outranks the target on priority and owns no model.
			seedConn(t, s, "decoy-conn", decoy.URL, "", 100)
			seedConn(t, s, "target-conn", target.URL, "", 1)
			seedModel(t, s, "target-conn", tc.model)

			resp := nonChatPost(t, ts, tc.endpoint, tc.body)
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
			}

			if hits := decoy.hits(); len(hits) != 0 {
				t.Errorf("request routed to the wrong connection: decoy received %v", hits)
			}
			hits := target.hits()
			if len(hits) != 1 {
				t.Fatalf("expected exactly 1 request on the model's connection, got %v", hits)
			}
			if hits[0] != tc.wantPath {
				t.Errorf("upstream path = %q, want %q", hits[0], tc.wantPath)
			}
		})
	}
}

// Every catalogue preset stores a base URL ending in "/v1". Appending the
// canonical "/v1/..." path to that must not yield "/v1/v1/...".
func TestNonChatVersionedBaseDoesNotDoubleVersionSegment(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		model    string
		body     string
		wantPath string
	}{
		{
			name:     "images",
			endpoint: "/v1/images/generations",
			model:    "img-v1",
			body:     `{"model":"img-v1","prompt":"x"}`,
			wantPath: "/v1/images/generations",
		},
		{
			name:     "audio speech",
			endpoint: "/v1/audio/speech",
			model:    "tts-v1",
			body:     `{"model":"tts-v1","input":"x"}`,
			wantPath: "/v1/audio/speech",
		},
		{
			name:     "embeddings",
			endpoint: "/v1/embeddings",
			model:    "emb-v1",
			body:     `{"model":"emb-v1","input":"x"}`,
			wantPath: "/v1/embeddings",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, ts := newTestServer(t, &config.Config{MasterKey: nonChatMasterKey})
			up := newRecordingUpstream(t, `{"ok":true}`)

			// The versioned base shape used by every preset in seedCatalogue().
			seedConn(t, s, "v1-conn", up.URL, "/v1", 10)
			seedModel(t, s, "v1-conn", tc.model)

			resp := nonChatPost(t, ts, tc.endpoint, tc.body)
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
			}

			hits := up.hits()
			if len(hits) != 1 {
				t.Fatalf("expected 1 upstream request, got %v", hits)
			}
			if strings.Contains(hits[0], "/v1/v1") {
				t.Errorf("doubled version segment in upstream path: %q", hits[0])
			}
			if hits[0] != tc.wantPath {
				t.Errorf("upstream path = %q, want %q", hits[0], tc.wantPath)
			}
		})
	}
}

// Audio transcriptions send multipart/form-data, so the model arrives as a form
// field rather than JSON. Routing must still follow it, and the multipart body
// must reach the upstream unchanged.
func TestAudioTranscriptionsRoutesByMultipartModelField(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: nonChatMasterKey})

	decoy := newRecordingUpstream(t, `{"decoy":true}`)
	target := newRecordingUpstream(t, `{"text":"hello"}`)
	seedConn(t, s, "decoy-conn", decoy.URL, "/v1", 100)
	seedConn(t, s, "stt-conn", target.URL, "/v1", 1)
	seedModel(t, s, "stt-conn", "whisper-x")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("model", "whisper-x"); err != nil {
		t.Fatalf("write model field: %v", err)
	}
	fw, err := mw.CreateFormFile("file", "a.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte("RIFFfake-audio-bytes")); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	sent := buf.Bytes()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/audio/transcriptions", bytes.NewReader(sent))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+nonChatMasterKey)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST transcriptions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}

	if hits := decoy.hits(); len(hits) != 0 {
		t.Errorf("multipart request routed to the wrong connection: %v", hits)
	}
	hits := target.hits()
	if len(hits) != 1 {
		t.Fatalf("expected 1 upstream request on the model's connection, got %v", hits)
	}
	if hits[0] != "/v1/audio/transcriptions" {
		t.Errorf("upstream path = %q, want %q", hits[0], "/v1/audio/transcriptions")
	}
}

// Backward compatibility: a body naming no model (or an unknown one) must keep
// the pre-change behaviour of falling back to the first active connection,
// rather than turning a previously-working request into a 404.
func TestNonChatFallsBackToFirstConnectionWhenModelUnknown(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: nonChatMasterKey})
	first := newRecordingUpstream(t, `{"ok":true}`)
	seedConn(t, s, "only-conn", first.URL, "/v1", 10)

	for _, body := range []string{
		`{"prompt":"no model field"}`,
		`{"model":"model-nobody-has","prompt":"x"}`,
	} {
		resp := nonChatPost(t, ts, "/v1/images/generations", body)
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("body %s: expected 200, got %d: %s", body, resp.StatusCode, b)
		}
	}

	hits := first.hits()
	if len(hits) != 2 {
		t.Fatalf("expected both requests to fall back to the only connection, got %v", hits)
	}
	for _, h := range hits {
		if h != "/v1/images/generations" {
			t.Errorf("fallback path = %q, want %q", h, "/v1/images/generations")
		}
	}
}

// The request body must reach the upstream byte-for-byte: routing reads the
// model out of it, and a naive implementation that consumes the reader would
// forward an empty body.
func TestNonChatForwardsBodyUnchanged(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: nonChatMasterKey})

	var got []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)

	seedConn(t, s, "body-conn", up.URL, "/v1", 10)
	seedModel(t, s, "body-conn", "img-body")

	body := `{"model":"img-body","prompt":"a detailed prompt","n":2}`
	resp := nonChatPost(t, ts, "/v1/images/generations", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(got) != body {
		t.Errorf("upstream body mismatch:\n got=%s\nwant=%s", got, body)
	}
}
