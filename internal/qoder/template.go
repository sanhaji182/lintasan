package qoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Request-template provisioning.
//
// The Qoder chat request is built from a large fixed document containing the
// client's system instructions and its tool schemas. That document is the
// vendor's content — it is not code, it is not covered by this repository's
// license, and it was obtained by inspecting a commercial product rather than
// being published for reuse.
//
// It is therefore NOT committed here. The operator provisions it locally, and
// the package refuses to build a request until they do. Two consequences worth
// stating plainly:
//
//   - The package is inert without provisioning. That is the intended default for
//     an Experimental provider whose upstream contract is re-derived: nothing
//     runs until someone deliberately supplies the inputs.
//   - Nothing about the wire contract is lost. The STRUCTURE of the document (the
//     field names, the nesting, which fields upstream reads for the prompt) is
//     protocol knowledge and is encoded in BuildChatBody and asserted by tests.
//     Only the vendor's prose and tool descriptions are withheld.

// TemplateEnvVar names the environment variable holding the path to the request
// template. Provisioning is a filesystem operation rather than configuration so
// the content never has to travel through a settings table or an API.
const TemplateEnvVar = "LINTASAN_QODER_TEMPLATE"

// ErrTemplateUnavailable is returned by every request-building path until a
// template has been installed. It is a distinct, named error so callers can
// distinguish "this deployment has not provisioned Qoder" from a malformed
// request.
var ErrTemplateUnavailable = errors.New(
	"qoder: request template not provisioned — set " + TemplateEnvVar +
		" to the path of the client request template, or call LoadTemplate/SetTemplate")

// installedTemplate holds the parsed template. It is written at most once during
// setup and read on every request, so it is an atomic pointer rather than a
// mutex-guarded map: reads are lock-free and a torn read is impossible.
var installedTemplate atomic.Pointer[map[string]any]

// SetTemplate parses raw template bytes and installs them.
//
// The stored document is not valid JSON as-is: the timestamp placeholder sits
// unquoted, because the vendor's client substitutes it textually before sending.
// The placeholders are replaced with type-correct literals here so the document
// parses; every real value is then written structurally by BuildChatBody.
func SetTemplate(raw []byte) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fmt.Errorf("qoder: template is empty")
	}

	normalized := strings.NewReplacer(
		"{UUID1}", "00000000-0000-0000-0000-000000000001",
		"{UUID2}", "00000000-0000-0000-0000-000000000002",
		"{UUID3}", "00000000-0000-0000-0000-000000000003",
		"{UUID4}", "00000000-0000-0000-0000-000000000004",
		"{UUID5}", "00000000-0000-0000-0000-000000000005",
		"{TIME1}", "0",
	).Replace(string(raw))

	var parsed map[string]any
	if err := json.Unmarshal([]byte(normalized), &parsed); err != nil {
		return fmt.Errorf("qoder: template is not parseable: %w", err)
	}
	// A template that parses but lacks the structural pieces BuildChatBody writes
	// to would fail on every request with a confusing error, so the shape is
	// validated once at load instead.
	for _, key := range []string{"chat_context", "model_config", "business", "messages", "parameters"} {
		if _, ok := parsed[key]; !ok {
			return fmt.Errorf("qoder: template is missing the %q object", key)
		}
	}

	installedTemplate.Store(&parsed)
	return nil
}

// LoadTemplate reads a template file and installs it.
func LoadTemplate(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("qoder: read template %q: %w", path, err)
	}
	return SetTemplate(raw)
}

// InitTemplateFromEnv installs the template named by TemplateEnvVar, if it is
// set. An unset variable is not an error: the package stays inert and reports
// ErrTemplateUnavailable per request, which is the correct state for a
// deployment that has not provisioned Qoder.
func InitTemplateFromEnv() error {
	path := strings.TrimSpace(os.Getenv(TemplateEnvVar))
	if path == "" {
		return nil
	}
	return LoadTemplate(path)
}

// TemplateReady reports whether a template has been installed.
func TemplateReady() bool { return installedTemplate.Load() != nil }

// requestTemplate returns the installed template.
func requestTemplate() (map[string]any, error) {
	t := installedTemplate.Load()
	if t == nil {
		return nil, ErrTemplateUnavailable
	}
	return *t, nil
}
