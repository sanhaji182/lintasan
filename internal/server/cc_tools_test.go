package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// CommandCode Alpha expects Anthropic-style tool declarations:
//
//	{name, description, input_schema}
//
// 9router's openai-to-commandcode.js:190-210 (convertTools) and the Go fork's
// commandcodeTools() agree on this projection. OpenAI-shaped
// {type:"function", function:{name, description, parameters}} must be
// converted; passing it through verbatim makes CommandCode reject the request
// with "expected string, received undefined at params.tools[0].name".
func TestTransformForCommandCode_ProjectsOpenAITools(t *testing.T) {
	body := []byte(`{
		"model": "deepseek-v4-pro",
		"messages": [{"role": "user", "content": "hi"}],
		"tools": [
			{"type":"function","function":{
				"name":"get_weather",
				"description":"Get weather for a city",
				"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
			}},
			{"name":"native_tool","description":"already projected","input_schema":{"type":"object"}}
		],
		"max_tokens": 16
	}`)

	out := transformForCommandCode(body, "auto")
	var req struct {
		Params struct {
			Tools []map[string]any `json:"tools"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(req.Params.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d (%s)", len(req.Params.Tools), out)
	}

	first := req.Params.Tools[0]
	if first["type"] != nil || first["function"] != nil {
		t.Errorf("OpenAI wrapper leaked into projected tool: %v", first)
	}
	if first["name"] != "get_weather" || first["description"] != "Get weather for a city" {
		t.Errorf("name/description not projected: %v", first)
	}
	schema, ok := first["input_schema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Errorf("parameters not projected to input_schema: %v", first)
	}

	second := req.Params.Tools[1]
	if second["name"] != "native_tool" {
		t.Errorf("Anthropic-shaped tool mangled: %v", second)
	}
}

func TestTransformForCommandCode_NonFunctionToolShapes(t *testing.T) {
	// Tools that are neither OpenAI function nor Anthropic-shaped must be
	// dropped, mirroring convertTools' "skip unrecognised" behaviour rather
	// than forwarding an invalid declaration.
	body := []byte(`{
		"model": "core",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [
			{"type":"function"},
			{"type":"web_search_20250305","name":"web_search"},
			null
		],
		"max_tokens": 16
	}`)
	out := transformForCommandCode(body, "auto")
	var req struct {
		Params struct {
			Tools []map[string]any `json:"tools"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	for _, tool := range req.Params.Tools {
		if strings.TrimSpace(fieldsToString(tool)) == "" {
			continue
		}
		if _, ok := tool["name"]; !ok {
			t.Errorf("unrecognised tool forwarded: %v", tool)
		}
	}
}

func fieldsToString(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}
