package server

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// chat_vision_test.go — cover for image attachments reaching the model.
//
// Before this, uploads of PNG/JPEG were stored and then rejected with "this
// gateway does not forward them to vision models yet", so "upload a screenshot
// and ask about it" simply did not work.

// a 1x1 transparent PNG, the smallest valid image.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func writeTempImage(t *testing.T, mime string) *conversation.Attachment {
	t.Helper()
	dir := t.TempDir()
	raw, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	p := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return &conversation.Attachment{
		ID:       "a_test",
		Filename: "shot.png",
		Mime:     mime,
		Size:     int64(len(raw)),
		Path:     p,
	}
}

func TestImagePartForAttachment_ProducesDataURL(t *testing.T) {
	a := writeTempImage(t, "image/png")

	part, ok := imagePartForAttachment(a)
	if !ok {
		t.Fatal("image was not converted to a vision part")
	}
	if part["type"] != "image_url" {
		t.Fatalf("expected type image_url, got %v", part["type"])
	}
	iu, ok := part["image_url"].(map[string]any)
	if !ok {
		t.Fatal("image_url is not a map")
	}
	url, _ := iu["url"].(string)
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("expected a base64 data URL, got %.40s", url)
	}
	// The payload must decode back to the original bytes.
	enc := strings.TrimPrefix(url, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}
	orig, _ := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if len(decoded) != len(orig) {
		t.Fatalf("payload length %d != original %d", len(decoded), len(orig))
	}
}

func TestImagePartForAttachment_RejectsNonImage(t *testing.T) {
	a := writeTempImage(t, "text/plain")
	if _, ok := imagePartForAttachment(a); ok {
		t.Fatal("a non-image attachment must not become a vision part")
	}
}

func TestImagePartForAttachment_RejectsMissingFile(t *testing.T) {
	a := &conversation.Attachment{ID: "a_x", Mime: "image/png", Path: "/nonexistent/nope.png"}
	if _, ok := imagePartForAttachment(a); ok {
		t.Fatal("a missing file must not produce a vision part")
	}
}

func TestImagePartForAttachment_RejectsEmptyPath(t *testing.T) {
	a := &conversation.Attachment{ID: "a_x", Mime: "image/png"}
	if _, ok := imagePartForAttachment(a); ok {
		t.Fatal("an attachment with no path must not produce a vision part")
	}
}

func TestImageMIMEIsNotTreatedAsExtractionFailure(t *testing.T) {
	// The upload handler must not report a warning for a stored image.
	_, err := extractText("", "image/png", "shot.png")
	if err == nil {
		t.Fatal("expected the image sentinel")
	}
	if !errors.Is(err, errImageNeedsVision) {
		t.Fatalf("expected errImageNeedsVision, got %v", err)
	}
}

func TestAttachImagesToLastUserTurn_ConvertsOnlyFinalUserMessage(t *testing.T) {
	wire := []map[string]any{
		{"role": "system", "content": "you are helpful"},
		{"role": "user", "content": "first question"},
		{"role": "assistant", "content": "first answer"},
		{"role": "user", "content": "what is in this screenshot?"},
	}
	images := []map[string]any{{
		"type":      "image_url",
		"image_url": map[string]any{"url": "data:image/png;base64,AAAA"},
	}}

	out := attachImagesToLastUserTurn(wire, images)
	if len(out) != len(wire) {
		t.Fatalf("message count changed: %d -> %d", len(wire), len(out))
	}

	// The earlier user turn stays a plain string.
	if _, ok := out[1]["content"].(string); !ok {
		t.Fatal("earlier user turn must stay a string, not become multimodal")
	}

	parts, ok := out[3]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("last user turn was not converted to parts: %T", out[3]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("expected text + image, got %d parts", len(parts))
	}
	if parts[0]["type"] != "text" || parts[0]["text"] != "what is in this screenshot?" {
		t.Fatalf("text part wrong: %v", parts[0])
	}
	if parts[1]["type"] != "image_url" {
		t.Fatalf("image part wrong: %v", parts[1])
	}

	// Original slice must be untouched (we copy, not mutate).
	if _, ok := wire[3]["content"].(string); !ok {
		t.Fatal("input wire slice was mutated")
	}
}

func TestAttachImagesToLastUserTurn_SkipsEmptyText(t *testing.T) {
	wire := []map[string]any{{"role": "user", "content": "   "}}
	images := []map[string]any{{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}}}

	out := attachImagesToLastUserTurn(wire, images)
	parts, ok := out[0]["content"].([]map[string]any)
	if !ok {
		t.Fatal("not converted")
	}
	if len(parts) != 1 {
		t.Fatalf("blank text should not produce a text part, got %d parts", len(parts))
	}
}

func TestAttachImagesToLastUserTurn_NoImagesIsNoop(t *testing.T) {
	wire := []map[string]any{{"role": "user", "content": "hi"}}
	out := attachImagesToLastUserTurn(wire, nil)
	if _, ok := out[0]["content"].(string); !ok {
		t.Fatal("no-op path must not convert content")
	}
}
