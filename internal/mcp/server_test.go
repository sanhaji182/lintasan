package mcp

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	s := NewServer("test", "1.0.0")
	if s.name != "test" {
		t.Errorf("expected name 'test', got '%s'", s.name)
	}
}

func TestHandleInitialize(t *testing.T) {
	s := NewServer("lintasan", "2.2.0")
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("unexpected protocol version: %v", result["protocolVersion"])
	}
}

func TestHandlePing(t *testing.T) {
	s := NewServer("test", "1.0.0")
	req := &Request{JSONRPC: "2.0", ID: 1, Method: "ping"}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	s := NewServer("test", "1.0.0")
	req := &Request{JSONRPC: "2.0", ID: 1, Method: "unknown"}
	resp := s.HandleJSONRPC(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestRegisterAndListTools(t *testing.T) {
	s := NewServer("test", "1.0.0")
	s.RegisterTool(Tool{
		Name:        "test.tool",
		Description: "A test tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(params map[string]any) (any, error) {
		return "ok", nil
	})

	req := &Request{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestCallTool(t *testing.T) {
	s := NewServer("test", "1.0.0")
	s.RegisterTool(Tool{
		Name:        "echo",
		Description: "Echo input",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
	}, func(params map[string]any) (any, error) {
		return map[string]any{"echo": params["text"]}, nil
	})

	paramsJSON := `{"name":"echo","arguments":{"text":"hello"}}`
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  []byte(paramsJSON),
	}
	resp := s.HandleJSONRPC(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestCallToolNotFound(t *testing.T) {
	s := NewServer("test", "1.0.0")
	paramsJSON := `{"name":"nonexistent","arguments":{}}`
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  []byte(paramsJSON),
	}
	resp := s.HandleJSONRPC(req)
	if resp.Error == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestMultipleTools(t *testing.T) {
	s := NewServer("test", "1.0.0")
	tools := []string{"tool1", "tool2", "tool3"}
	for _, name := range tools {
		s.RegisterTool(Tool{
			Name:        name,
			Description: "Tool " + name,
			InputSchema: map[string]any{"type": "object"},
		}, func(params map[string]any) (any, error) {
			return "ok", nil
		})
	}

	if len(s.tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(s.tools))
	}
}

func TestToolHandlerError(t *testing.T) {
	s := NewServer("test", "1.0.0")
	s.RegisterTool(Tool{
		Name:        "fail",
		Description: "Always fails",
		InputSchema: map[string]any{"type": "object"},
	}, func(params map[string]any) (any, error) {
		return nil, &testError{"intentional error"}
	})

	paramsJSON := `{"name":"fail","arguments":{}}`
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  []byte(paramsJSON),
	}
	resp := s.HandleJSONRPC(req)
	// Should not have JSON-RPC error, but tool error in result
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
