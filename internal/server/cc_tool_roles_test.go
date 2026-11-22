package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// CommandCode Alpha's params.messages schema accepts only "user" and
// "assistant" roles. A verbatim pass-through of OpenAI-style `role: "tool"`
// messages makes every follow-up turn in a tool-using conversation fail with
// `400 ... expected one of "user"|"assistant" at params.messages[N].role`.
//
// The transformer must fold tool results into a user-role message so that a
// session with tool history keeps working on a commandcode connection.
func TestTransformForCommandCode_ToolRolesFoldedIntoUser(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-pro",
		"messages": [
			{"role": "system", "content": "Be concise."},
			{"role": "user", "content": "What is 2+2?"},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "calc", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "4"}
		],
		"max_tokens": 16
	}`)

	out := transformForCommandCode(body, "auto")

	var req struct {
		Params struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			System string `json:"system"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("transform output not valid JSON: %v\n%s", err, out)
	}

	if req.Params.System != "Be concise." {
		t.Errorf("system prompt lost: got %q", req.Params.System)
	}

	roles := make([]string, 0, len(req.Params.Messages))
	for _, m := range req.Params.Messages {
		roles = append(roles, m.Role)
		if m.Role == "tool" || m.Role == "function" {
			t.Errorf("commandcode-incompatible role %q leaked into params.messages", m.Role)
		}
	}
	t.Logf("transformed roles: %v", roles)

	// The tool result text must survive the fold, not be dropped.
	joined := ""
	for _, m := range req.Params.Messages {
		if s, ok := m.Content.(string); ok {
			joined += s + "\n"
		}
	}
	if !strings.Contains(joined, "4") {
		t.Errorf("tool result content lost during fold; messages content = %q", joined)
	}
}

func TestTransformForCommandCode_AssistantToolCallsPreserved(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-pro",
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "calc", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "4"},
			{"role": "user", "content": "thanks"}
		],
		"max_tokens": 16
	}`)

	out := transformForCommandCode(body, "auto")
	var req struct {
		Params struct {
			Messages []map[string]any `json:"messages"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}

	// Non-tool messages must keep their original roles and order.
	var roles []string
	for _, m := range req.Params.Messages {
		roles = append(roles, m["role"].(string))
	}
	want := []string{"user", "assistant", "user", "user"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("unexpected roles: got %v, want %v", roles, want)
	}

	// Assistant tool_calls must still be present for the upstream to see.
	found := false
	for _, m := range req.Params.Messages {
		if m["role"] == "assistant" && m["tool_calls"] != nil {
			found = true
		}
	}
	if !found {
		t.Errorf("assistant tool_calls were stripped")
	}
}
