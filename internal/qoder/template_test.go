package qoder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testTemplate is a MINIMAL request skeleton carrying the structure BuildChatBody
// depends on, with none of the vendor's content.
//
// Tests must not depend on the real template: it contains the vendor's system
// instructions and tool schemas, which are deliberately not part of this
// repository. What the tests need is the SHAPE — the objects that get written to
// and the nesting the prompt is echoed into — and that shape is protocol
// knowledge, so it belongs here.
//
// `installTestTemplate` is called by the tests that exercise body building. The
// provisioning tests below it assert the opposite behaviour: that the package
// refuses to build anything when no template is installed.
const testTemplate = `{
  "request_id": "{UUID1}",
  "request_set_id": "{UUID2}",
  "chat_record_id": "{UUID3}",
  "stream": true,
  "chat_task": "FREE_INPUT",
  "chat_context": {
    "chatPrompt": "",
    "extra": {
      "context": [],
      "modelConfig": {"is_reasoning": false, "key": "auto"},
      "originalContent": {"type": "text", "text": "placeholder"}
    },
    "features": [],
    "imageUrls": null,
    "text": {"type": "text", "text": "placeholder"}
  },
  "is_reply": true,
  "is_retry": false,
  "session_id": "{UUID4}",
  "code_language": "",
  "source": 1,
  "version": "3",
  "chat_prompt": "",
  "parameters": {"max_tokens": 16384},
  "aliyun_user_type": "personal_standard",
  "session_type": "qoder",
  "agent_id": "agent_common",
  "task_id": "common",
  "model_config": {
    "key": "auto",
    "display_name": "Auto",
    "model": "",
    "format": "openai",
    "is_vl": false,
    "is_reasoning": false,
    "api_key": "",
    "url": "",
    "source": "system",
    "max_input_tokens": 180000
  },
  "messages": [
    {"role": "system", "content": "TEST SYSTEM INSTRUCTIONS"}
  ],
  "tools": [],
  "business": {"product": "ide", "version": "1.1.3", "type": "agent", "id": "{UUID5}", "name": "placeholder", "begin_at": {TIME1}, "stage": "start"}
}`

// installTestTemplate provisions the minimal template for a test and registers
// cleanup that removes it again, so template state cannot leak between tests.
func installTestTemplate(t *testing.T) {
	t.Helper()
	if err := SetTemplate([]byte(testTemplate)); err != nil {
		t.Fatalf("install test template: %v", err)
	}
	t.Cleanup(func() { installedTemplate.Store(nil) })
}

// clearTemplate removes any installed template for the duration of a test.
func clearTemplate(t *testing.T) {
	t.Helper()
	prev := installedTemplate.Load()
	installedTemplate.Store(nil)
	t.Cleanup(func() {
		if prev != nil {
			installedTemplate.Store(prev)
		}
	})
}

// TestUnprovisionedPackageRefusesToBuild is the guard on the default state.
//
// An Experimental provider whose upstream protocol is re-derived should be inert
// until someone provisions it. If this ever starts succeeding, a deployment that
// never opted in would begin constructing requests.
func TestUnprovisionedPackageRefusesToBuild(t *testing.T) {
	clearTemplate(t)

	if TemplateReady() {
		t.Fatal("package reports a template as ready with none installed")
	}
	_, err := BuildChatBody(ChatRequest{
		Model:    "auto",
		Messages: []map[string]any{{"role": "user", "content": "hi"}},
	})
	if err == nil {
		t.Fatal("BuildChatBody succeeded without a provisioned template")
	}
	if err != ErrTemplateUnavailable {
		t.Errorf("error should be the named ErrTemplateUnavailable, got: %v", err)
	}
	// The error must tell an operator what to do about it.
	for _, want := range []string{TemplateEnvVar, "template"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message does not mention %q: %v", want, err)
		}
	}
}

// TestSetTemplateValidatesShape rejects templates that parse but cannot be built
// from. Without this, a truncated template would fail on every request instead of
// once at load.
func TestSetTemplateValidatesShape(t *testing.T) {
	clearTemplate(t)

	if err := SetTemplate([]byte(`{}`)); err == nil {
		t.Error("expected an empty object to be rejected")
	}
	if err := SetTemplate([]byte(``)); err == nil {
		t.Error("expected empty input to be rejected")
	}
	if err := SetTemplate([]byte(`{not json`)); err == nil {
		t.Error("expected malformed JSON to be rejected")
	}

	// A template missing one required object must be rejected by name.
	partial := strings.Replace(testTemplate, `"business":`, `"business_renamed":`, 1)
	if err := SetTemplate([]byte(partial)); err == nil {
		t.Error("expected a template without `business` to be rejected")
	} else if !strings.Contains(err.Error(), "business") {
		t.Errorf("error should name the missing object, got: %v", err)
	}

	// And a good one must be accepted afterwards.
	if err := SetTemplate([]byte(testTemplate)); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	if !TemplateReady() {
		t.Error("TemplateReady false after a successful SetTemplate")
	}
}

// TestLoadTemplateFromFile covers the filesystem provisioning path and its
// failure mode.
func TestLoadTemplateFromFile(t *testing.T) {
	clearTemplate(t)

	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, []byte(testTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadTemplate(path); err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if !TemplateReady() {
		t.Error("template not ready after LoadTemplate")
	}

	if err := LoadTemplate(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("expected a missing file to be an error")
	}
}

// TestInitTemplateFromEnv covers the deployment path: env var set loads, env var
// absent is not an error.
func TestInitTemplateFromEnv(t *testing.T) {
	clearTemplate(t)

	t.Setenv(TemplateEnvVar, "")
	if err := InitTemplateFromEnv(); err != nil {
		t.Errorf("an unset %s must not be an error, got: %v", TemplateEnvVar, err)
	}
	if TemplateReady() {
		t.Error("no template should be installed when the env var is unset")
	}

	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, []byte(testTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(TemplateEnvVar, path)
	if err := InitTemplateFromEnv(); err != nil {
		t.Fatalf("InitTemplateFromEnv with a valid path: %v", err)
	}
	if !TemplateReady() {
		t.Error("template not ready after InitTemplateFromEnv")
	}
}

// TestRepoDoesNotContainVendorTemplate enforces the licensing decision.
//
// The vendor's request template — system instructions and tool schemas — must not
// be committed. This repository is public and the content is not ours to
// redistribute. A future contributor "helpfully" re-adding it as an embed is
// exactly the regression this catches.
func TestRepoDoesNotContainVendorTemplate(t *testing.T) {
	// The package source directory must not carry the template file.
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), "baseprompt.json") {
			t.Fatalf("vendor request template %q is present in the package; it must be operator-provisioned, not committed", e.Name())
		}
	}

	// Nor may any source file embed one.
	src, err := os.ReadFile("chat.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "go:embed") {
		t.Error("chat.go embeds a file; the request template must not be embedded in a public repository")
	}
}

// TestTemplatePlaceholdersAreTolerated confirms the timestamp placeholder does not
// have to be valid JSON before substitution — the vendor ships it unquoted, and a
// strict loader would reject every real template.
func TestTemplatePlaceholdersAreTolerated(t *testing.T) {
	clearTemplate(t)

	unquoted := strings.Replace(testTemplate, `"begin_at": {TIME1}`, `"begin_at": {TIME1}`, 1)
	if !strings.Contains(unquoted, "{TIME1}") {
		t.Fatal("fixture no longer exercises the unquoted placeholder")
	}
	// Raw is invalid JSON as shipped.
	if json.Valid([]byte(unquoted)) {
		t.Skip("fixture happens to be valid JSON; placeholder handling not exercised")
	}
	if err := SetTemplate([]byte(unquoted)); err != nil {
		t.Fatalf("unquoted {TIME1} placeholder should be tolerated: %v", err)
	}
}
