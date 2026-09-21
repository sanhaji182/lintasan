package qoder

import (
	"encoding/json"
	"testing"
)

// capturedModelList is a real upstream model-list response, reduced to three
// entries. It is kept verbatim — including the fields this package ignores —
// because its value is as a shape fixture, not as a sample of the models one
// particular account happens to have.
//
// The fixture exists because of a specific incident: `price_factor` is a float
// on the wire, and typing it as a string in the Model struct did not fail one
// field, it failed decoding of the WHOLE response. Discovery returned zero models
// for every account, and the symptom looked like an auth problem rather than a
// struct-tag problem. Any future field-type change now fails here first.
const capturedModelList = `{
  "chat": [],
  "assistant": [
    {
      "key": "auto",
      "format": "openai",
      "source": "system",
      "enable": true,
      "display_name": "Auto",
      "is_vl": true,
      "is_reasoning": false,
      "is_default": true,
      "price_factor": 0.5,
      "max_input_tokens": 200000,
      "strategies": [{"tag": "C4", "priority": 999, "enabled": false}],
      "is_sensitive": true
    },
    {
      "key": "qmodel_38max",
      "format": "openai",
      "source": "system",
      "enable": true,
      "display_name": "Qoder 38 Max",
      "is_vl": false,
      "is_reasoning": true,
      "is_default": false,
      "price_factor": 1.0,
      "max_input_tokens": 180000,
      "max_output_tokens": 32768,
      "strategies": [],
      "is_sensitive": false
    },
    {
      "key": "retired_model",
      "format": "openai",
      "source": "system",
      "enable": false,
      "display_name": "Retired",
      "is_vl": false,
      "is_reasoning": false,
      "is_default": false,
      "price_factor": 0.25,
      "max_input_tokens": 32000
    }
  ],
  "inline": [{"key": "inline_only", "enable": true, "price_factor": 1}],
  "experts": [],
  "nap": [],
  "qwork": [],
  "qwake": [],
  "quest": [],
  "app": [],
  "byok_teams": [],
  "byok_enterprise": []
}`

// TestParseCapturedModelList decodes the real response shape. This is the guard
// against a single mismatched field type silently emptying the model catalogue.
func TestParseCapturedModelList(t *testing.T) {
	models, err := parseModelList([]byte(capturedModelList))
	if err != nil {
		t.Fatalf("failed to decode the captured upstream response: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 enabled assistant models: %v", len(models), ModelKeys(models))
	}

	byKey := map[string]Model{}
	for _, m := range models {
		byKey[m.Key] = m
	}

	auto, ok := byKey["auto"]
	if !ok {
		t.Fatal("'auto' missing")
	}
	if auto.PriceFactor != 0.5 {
		t.Errorf("price_factor = %v, want 0.5 (must decode as a float, not a string)", auto.PriceFactor)
	}
	if !auto.IsDefault {
		t.Error("is_default not read")
	}
	if !auto.IsVision {
		t.Error("is_vl not read")
	}
	if auto.MaxInputTokens != 200000 {
		t.Errorf("max_input_tokens = %d, want 200000", auto.MaxInputTokens)
	}

	reasoning, ok := byKey["qmodel_38max"]
	if !ok {
		t.Fatal("'qmodel_38max' missing")
	}
	if !reasoning.IsReasoning {
		t.Error("is_reasoning not read")
	}
	if reasoning.MaxOutputTokens != 32768 {
		t.Errorf("max_output_tokens = %d, want 32768", reasoning.MaxOutputTokens)
	}

	// A disabled model must not be advertised: routing to it would fail at
	// request time and look like a router bug rather than an entitlement.
	if _, ok := byKey["retired_model"]; ok {
		t.Error("disabled model was included")
	}
	// Only the assistant group is chat-capable here.
	if _, ok := byKey["inline_only"]; ok {
		t.Error("inline-surface model was included; it is not chat-capable")
	}
}

// TestModelJSONRoundTrip locks the struct tags so a rename is caught even if the
// parse test is adjusted.
func TestModelJSONRoundTrip(t *testing.T) {
	const one = `{"key":"k","display_name":"D","price_factor":2.5,"is_reasoning":true,"max_input_tokens":1000}`
	var m Model
	if err := json.Unmarshal([]byte(one), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Key != "k" || m.DisplayName != "D" || m.PriceFactor != 2.5 || !m.IsReasoning || m.MaxInputTokens != 1000 {
		t.Errorf("field mapping is wrong: %+v", m)
	}
}
