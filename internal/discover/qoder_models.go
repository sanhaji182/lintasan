package discover

// qoder_models.go — model discovery for Qoder connections.
//
// Qoder's catalogue cannot be read with the generic GET {base_url}/v1/models:
// the endpoint is a signed, envelope-encoded call to a different host than the
// one the connection points at, and the answer is specific to the credential
// used. Entitlements differ between accounts on the same upstream (observed: 2
// models versus 15), so discovery is inherently per-connection.
//
// The credential comes from the connection row itself, which is also how the
// live request path gets it, so discovery and serving cannot disagree about which
// account is being described.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// fetchQoderModels reads the catalogue for a Qoder connection.
func (d *Discoverer) fetchQoderModels(conn map[string]any) ([]ModelInfo, error) {
	if !qoder.TemplateReady() {
		// The template is provisioned per process by initQoder when the provider is
		// enabled. Reaching here without it means the provider is inert, and
		// reporting zero models is more honest than reporting a stale catalogue.
		return nil, fmt.Errorf("qoder: provider is not enabled (no request template provisioned)")
	}

	apiKey, _ := conn["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("qoder: connection has no credential")
	}

	salt := strings.TrimSpace(os.Getenv("LINTASAN_QODER_SALT"))
	region := strings.TrimSpace(os.Getenv("LINTASAN_QODER_REGION"))
	if region == "" {
		region = "global"
	}

	// The discoverer's own client is used so the timeout policy is shared with
	// every other provider probe.
	manager := qoder.NewSessionManager(salt, region, d.httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	models, err := manager.ListModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("qoder: list models: %w", err)
	}

	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, ModelInfo{
			ID:      m.Key,
			Name:    m.Name(),
			OwnedBy: "qoder",
		})
	}
	return out, nil
}
