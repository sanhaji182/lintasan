package qoder

import (
	"encoding/json"
	"net/http"
	"testing"
)

// jsonString encodes s as a JSON string literal (local to this test file).
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// Regression test for the 2026-09-30 combo ("Core") incident: upstream delivers
// a queue state nested two levels deep — the envelope body is a code/message
// JSON string whose message is itself the queue-state JSON. A parser that only
// unwraps one level classifies the frame as a generic 403, IsQueued() returns
// false, and the failover treats it as a hard credential failure: the client
// receives a bare error frame instead of being routed to a working account.
//
// This is the exact body shape observed live:
//
//	{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,...,\\\"retryAfterSeconds\\\":30,...}\"}"}
//
// The double-nested body carries the real queue signal. Surface it.
func TestParseEnvelopeError_NestedQueueStateIsUnwrapped(t *testing.T) {
	// Level 3: the actual queue state as upstream sends it.
	l3 := `{"isQueued":true,"modelKey":"qmodel_38max","queueCount":0,"queueType":"p3","retryAfterSeconds":30,"serviceAvailable":false,"waitTime":30}`
	// Level 2: a code/message wrapper.
	l2 := `{"code":"10605","message":` + jsonString(l3) + `}`
	// Level 1: the envelope's body (what parseEnvelopeError unwraps first).
	body := `{"code":"403","message":` + jsonString(l2) + `}`
	// Level 0: the envelope itself (statusCodeValue + body: "l1").
	frame := `{"statusCodeValue":403,"body":` + jsonString(body) + `}`

	got := parseEnvelopeError([]byte(frame))
	if got == nil {
		t.Fatal("expected an UpstreamError, got nil")
	}
	if !got.IsQueued() {
		t.Fatalf("expected Queued=true, got error %s", got)
	}
	if got.Code != QueueErrorCode {
		t.Fatalf("expected Code=%q, got %q", QueueErrorCode, got.Code)
	}
	if got.RetryAfterSeconds != 30 {
		t.Fatalf("expected RetryAfterSeconds=30, got %d", got.RetryAfterSeconds)
	}
	if got.Status != http.StatusForbidden && got.Status != http.StatusOK {
		t.Fatalf("expected status 403 or 200, got %d", got.Status)
	}
}

// The single-level case must still work. This is the shape documented live in
// 2026-09-22: envelope body is just the queue struct, no nesting.
func TestParseEnvelopeError_SingleLevelQueueStateIsUnchanged(t *testing.T) {
	l3 := `{"isQueued":true,"modelKey":"kmodel_latest","queueCount":0,"queueType":"p3","retryAfterSeconds":30,"serviceAvailable":false,"waitTime":30}`
	body := `{"code":"10605","message":` + jsonString(l3) + `}`
	frame := `{"statusCodeValue":403,"body":` + jsonString(body) + `}`

	got := parseEnvelopeError([]byte(frame))
	if got == nil {
		t.Fatal("expected an UpstreamError, got nil")
	}
	if !got.IsQueued() {
		t.Fatalf("expected Queued=true, got error %s", got)
	}
	if got.Code != QueueErrorCode {
		t.Fatalf("expected Code=%q, got %q", QueueErrorCode, got.Code)
	}
	if got.RetryAfterSeconds != 30 {
		t.Fatalf("expected RetryAfterSeconds=30, got %d", got.RetryAfterSeconds)
	}
}

// Code 105 (login expired) must still be parsed correctly through the same
// loop — the loop must not classify a plain 105 as queued just because the
// body is non-queue-shaped.
func TestParseEnvelopeError_LoginExpiredCode105IsNotQueued(t *testing.T) {
	body := `{"code":"105","message":"Login expired"}`
	frame := `{"statusCodeValue":403,"body":` + jsonString(body) + `}`

	got := parseEnvelopeError([]byte(frame))
	if got == nil {
		t.Fatal("expected an UpstreamError, got nil")
	}
	if got.IsQueued() {
		t.Fatalf("expected Queued=false for code 105, got error %s", got)
	}
	if !got.IsLoginExpired() {
		t.Fatalf("expected IsLoginExpired=true for code 105, got %s", got)
	}
	if got.Code != "105" {
		t.Fatalf("expected Code=105, got %q", got.Code)
	}
}
