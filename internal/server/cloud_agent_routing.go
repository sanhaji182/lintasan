package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	providerKindLLM        = "llm"
	providerKindCloudAgent = "cloud_agent"
	hopliteConnectionID    = "hoplite-cloud-agent"
)

type cloudComboTarget struct {
	Model        string
	ConnectionID string
	Kind         string
}

func isCloudAgentModel(model string) bool {
	return strings.HasPrefix(model, "hoplite-agent/") || strings.HasPrefix(model, "hoplite-model/v1/") || strings.HasPrefix(model, "hoplite-model/v2/")
}

func isHopliteConnectionID(id string) bool {
	return id == hopliteConnectionID || strings.HasPrefix(id, hopliteConnectionID+"-")
}

// comboContainsCloudAgent recognizes capability kind from either the stable
// connection id/kind persisted in SQLite or the advertised model namespace.
// Model recognition preserves legacy combos created before provider_kind.
func comboContainsCloudAgent(db *sql.DB, combo map[string]any) bool {
	for _, rawEntry := range asSlice(combo["entries"]) {
		entry := asMap(rawEntry)
		if isCloudAgentModel(fmt.Sprint(entry["model"])) {
			return true
		}
		for _, rawID := range asSlice(entry["connection_ids"]) {
			id := fmt.Sprint(rawID)
			if isHopliteConnectionID(id) {
				return true
			}
			var kind string
			if db != nil && db.QueryRow(`SELECT COALESCE(provider_kind,'llm') FROM connections WHERE id=?`, id).Scan(&kind) == nil && kind == providerKindCloudAgent {
				return true
			}
		}
	}
	return false
}

func validateCloudAgentCombo(db *sql.DB, combo map[string]any) error {
	if comboContainsCloudAgent(db, combo) && fmt.Sprint(combo["strategy"]) != string("priority") {
		return fmt.Errorf("cloud-agent combos require priority strategy")
	}
	return nil
}

func (s *Server) cloudComboTargets(name string) ([]cloudComboTarget, bool) {
	combos := asSlice(s.getJSONSetting("combos", []any{}))
	for _, raw := range combos {
		comboMap := asMap(raw)
		comboName := fmt.Sprint(comboMap["name"])
		if comboName == "<nil>" || comboName == "" {
			comboName = fmt.Sprint(comboMap["provider"])
		}
		if comboName != name || !comboContainsCloudAgent(s.db.Conn(), comboMap) {
			continue
		}
		if validateCloudAgentCombo(s.db.Conn(), comboMap) != nil {
			return nil, true
		}
		var targets []cloudComboTarget
		for _, rawEntry := range asSlice(comboMap["entries"]) {
			entry := asMap(rawEntry)
			model := fmt.Sprint(entry["model"])
			for _, rawID := range asSlice(entry["connection_ids"]) {
				id := fmt.Sprint(rawID)
				kind := providerKindLLM
				_ = s.db.Conn().QueryRow(`SELECT COALESCE(provider_kind,'llm') FROM connections WHERE id=?`, id).Scan(&kind)
				if isCloudAgentModel(model) || isHopliteConnectionID(id) {
					kind = providerKindCloudAgent
				}
				targets = append(targets, cloudComboTarget{Model: model, ConnectionID: id, Kind: kind})
			}
		}
		return targets, true
	}
	return nil, false
}

func cloneRequestWithModel(r *http.Request, raw []byte, model string) (*http.Request, []byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}
	payload["model"] = model
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	return clone, body, nil
}

func copyBufferedResponse(dst http.ResponseWriter, src *bufferedResponseWriter) {
	for key, values := range src.Header() {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	status := src.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = dst.Write(src.body.Bytes())
}

// handleCloudAgentCombo owns only combos containing cloud-agent capabilities.
// A cloud-agent failure is eligible for fallback only until CreateThread returns
// an accepted thread id. After acceptance every outcome is terminal to avoid
// duplicate autonomous work and side effects.
func (s *Server) handleCloudAgentCombo(w http.ResponseWriter, r *http.Request, model string, stream bool, messages []hopliteChatMessage, raw []byte) bool {
	targets, found := s.cloudComboTargets(model)
	if !found {
		return false
	}
	if len(targets) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_combo", "cloud-agent combo is invalid or has no targets")
		return true
	}
	if stream {
		writeOpenAIError(w, http.StatusBadRequest, "unsupported_parameter", "combos containing Cloud Agents do not support streaming")
		return true
	}

	var last *bufferedResponseWriter
	for _, target := range targets {
		buf := &bufferedResponseWriter{header: make(http.Header)}
		if target.Kind == providerKindCloudAgent {
			if accountID, _, _, _, ok := parseHopliteRoutedModelID(target.Model); !ok || accountID != target.ConnectionID {
				last = buf
				writeOpenAIError(buf, http.StatusServiceUnavailable, "hoplite_account_mismatch", "Hoplite model does not belong to the selected account")
				continue
			}
			s.handleHopliteCompletion(buf, r, target.Model, false, messages)
			last = buf
			if buf.Header().Get("X-Lintasan-Agent-Accepted") == "true" || buf.Header().Get("X-Lintasan-Agent-Acceptance-Uncertain") == "true" || (buf.status >= 200 && buf.status < 300) {
				copyBufferedResponse(w, buf)
				return true
			}
			continue
		}

		clone, requestBody, err := cloneRequestWithModel(r, raw, target.Model)
		if err != nil {
			continue
		}
		conn, err := s.proxy.findConnectionByID(target.ConnectionID)
		if err != nil {
			continue
		}
		response, err := s.proxy.doUpstream(clone, conn, requestBody)
		if err != nil {
			continue
		}
		for key, values := range response.Header {
			for _, value := range values {
				buf.Header().Add(key, value)
			}
		}
		buf.WriteHeader(response.StatusCode)
		_, _ = io.Copy(buf, response.Body)
		response.Body.Close()
		last = buf
		if buf.status >= 200 && buf.status < 500 && buf.status != http.StatusTooManyRequests {
			copyBufferedResponse(w, buf)
			return true
		}
	}
	if last != nil {
		copyBufferedResponse(w, last)
	} else {
		writeOpenAIError(w, http.StatusBadGateway, "routes_exhausted", "all cloud-agent combo targets failed before acceptance")
	}
	return true
}
