package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func ccToolCallStream() string {
	return strings.Join([]string{
		`{"type":"start"}`,
		`{"type":"text-delta","text":"Let me check."}`,
		`{"type":"tool-call","toolCallId":"call_1","toolName":"get_weather","input":{"city":"Jakarta"}}`,
		`{"type":"finish-step","finishReason":"tool-calls"}`,
		`{"type":"finish","finishReason":"tool-calls"}`,
	}, "\n")
}

// CommandCode emits `tool-call` SSE events. An OpenAI-compatible gateway must
// translate them into message.tool_calls with an index and JSON-encoded
// function.arguments; otherwise the client sees finish_reason=tool-calls with
// tool_calls=null and cannot execute the tool.
func TestTranslateCCAlphaToOpenAI_ToolCalls(t *testing.T) {
	out := translateCCAlphaToOpenAI([]byte(ccToolCallStream()))
	var resp struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(resp.Choices) == 0 {
		t.Fatalf("no choices: %s", out)
	}
	msg := resp.Choices[0].Message
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool_call, got %d: %s", len(msg.ToolCalls), out)
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" || tc.Type != "function" || tc.Function.Name != "get_weather" {
		t.Errorf("wrong tool call identity: %+v", tc)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Errorf("arguments not a JSON string: %q", tc.Function.Arguments)
	} else if args["city"] != "Jakarta" {
		t.Errorf("wrong arguments: %v", args)
	}
}

func TestPipeCCAlphaStreamToOpenAI_ToolCalls(t *testing.T) {
	p := &ProxyHandler{}
	rec := httptest.NewRecorder()
	_, tokens := p.pipeCCAlphaStreamToOpenAI(bytes.NewBufferString(ccToolCallStream()+"\n"), rec, nil, "core")

	if tokens == 0 {
		t.Errorf("expected tokens > 0")
	}
	body := rec.Body.String()
	head := body
	if len(head) > 800 {
		head = head[:800]
	}
	if !strings.Contains(body, `"tool_calls"`) {
		t.Errorf("stream chunks missing tool_calls: %s", head)
	}
	if !strings.Contains(body, `"get_weather"`) {
		t.Errorf("stream chunks missing tool name: %s", head)
	}
	if !strings.Contains(body, `"arguments"`) {
		t.Errorf("stream chunks missing arguments: %s", head)
	}
	if !strings.HasSuffix(body, "data: [DONE]\n\n") {
		tail := body
		if len(tail) > 100 {
			tail = tail[len(tail)-100:]
		}
		t.Errorf("stream missing DONE terminator: %s", tail)
	}
}
