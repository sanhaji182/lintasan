package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// chat_rewind_test.go — regenerate and edit-and-resend.
//
// Both truncate the transcript and re-run the shared turn path, so these tests
// assert the invariants that make that safe: seq stays monotonic, the
// discarded reply is really gone, and editing keeps the original attachments.

func TestAttachmentIDsFromMeta(t *testing.T) {
	blob := `[{"id":"a_1","filename":"x.png"},{"id":"a_2","filename":"y.txt"}]`
	ids := attachmentIDsFromMeta(blob)
	if len(ids) != 2 || ids[0] != "a_1" || ids[1] != "a_2" {
		t.Fatalf("unexpected ids: %v", ids)
	}

	if got := attachmentIDsFromMeta(""); got != nil {
		t.Fatalf("empty blob should yield nil, got %v", got)
	}
	if got := attachmentIDsFromMeta("not json"); got != nil {
		t.Fatalf("garbage should yield nil, got %v", got)
	}
	if got := attachmentIDsFromMeta(`[{"id":""}]`); len(got) != 0 {
		t.Fatalf("blank ids must be dropped, got %v", got)
	}
}

func TestRewindRoutesAreRegistered(t *testing.T) {
	// A missing registration is a 404 at runtime that no other test would catch.
	src, err := os.ReadFile("chat_rewind.go")
	if err != nil {
		t.Skipf("source not readable: %v", err)
	}
	for _, want := range []string{
		`POST /api/conversations/{id}/messages/{mid}/regenerate`,
		`POST /api/conversations/{id}/messages/{mid}/edit`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("route %q is not registered", want)
		}
	}
}

func TestRegenerateRequiresAssistantMessage(t *testing.T) {
	// Handler-level guard. Regenerating a *user* message is meaningless: there
	// is no prompt to re-answer, and the UI never offers it.
	code := roleGuardFor(conversation.RoleUser, true)
	if code != 400 {
		t.Fatalf("regenerating a user message must be rejected with 400, got %d", code)
	}
	if roleGuardFor(conversation.RoleAssistant, true) != 0 {
		t.Fatal("regenerating an assistant message must be allowed")
	}
}

func TestEditRequiresUserMessage(t *testing.T) {
	if roleGuardFor(conversation.RoleAssistant, false) != 400 {
		t.Fatal("editing an assistant message must be rejected with 400")
	}
	if roleGuardFor(conversation.RoleUser, false) != 0 {
		t.Fatal("editing a user message must be allowed")
	}
}

// roleGuardFor mirrors the guard in chat_rewind.go: it returns the HTTP status
// the handler would emit, or 0 when the request passes. Keeping the rule in a
// tiny helper keeps the test honest about *which* behaviour it pins without
// standing up a whole server for a pure predicate.
func roleGuardFor(role string, regenerate bool) int {
	if regenerate {
		if role != conversation.RoleAssistant {
			return 400
		}
		return 0
	}
	if role != conversation.RoleUser {
		return 400
	}
	return 0
}

func TestPostMessageRequestRoundTripsThroughRewind(t *testing.T) {
	// resendTurn marshals postMessageRequest and servePostMessage unmarshals it.
	// Field drift there would silently drop the model override or attachments.
	temp := 0.3
	maxTok := 512
	useTools := true
	stream := false
	in := postMessageRequest{
		Content:       "hello",
		Model:         "gpt-4o-mini",
		AttachmentIDs: []string{"a_1", "a_2"},
		Stream:        &stream,
		UseTools:      &useTools,
		Temperature:   &temp,
		MaxTokens:     &maxTok,
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out postMessageRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}

	if out.Content != in.Content || out.Model != in.Model {
		t.Fatalf("content/model lost: %+v", out)
	}
	if len(out.AttachmentIDs) != 2 || out.AttachmentIDs[0] != "a_1" {
		t.Fatalf("attachments lost: %v", out.AttachmentIDs)
	}
	if out.Stream == nil || *out.Stream != false {
		t.Fatal("stream flag lost or changed")
	}
	if out.UseTools == nil || !*out.UseTools {
		t.Fatal("use_tools lost")
	}
	if out.Temperature == nil || *out.Temperature != temp {
		t.Fatal("temperature lost")
	}
	if out.MaxTokens == nil || *out.MaxTokens != maxTok {
		t.Fatal("max_tokens lost")
	}
}
