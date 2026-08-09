package server

import (
	"strings"
)

// responsesAPISettingKey is the dashboard/DB setting that gates POST /v1/responses.
const responsesAPISettingKey = "responses_api_enabled"

// responsesAPIEnabled reports whether the Codex Responses surface is live.
//
// Resolution order (mirrors the OAuth IDE lab gate in oauth_ide_gate.go):
//  1. the dashboard setting `responses_api_enabled`, when it is set to a
//     recognizable boolean — this lets an admin flip the surface on/off from
//     Dashboard → Settings → Experimental with NO server restart, so an
//     accidental enable is one click to undo instead of a DB edit + restart.
//  2. otherwise the value latched at startup in initProviderSDK (p.responsesAPI),
//     which preserves the original boot-time contract for deployments that
//     seeded the setting before this gate existed.
//
// Default remains FALSE: with no setting row and no startup latch, the route
// answers 404 and prod is byte-identical to a build without the surface.
func (p *ProxyHandler) responsesAPIEnabled() bool {
	if p.db != nil {
		if v, err := p.db.GetSetting(responsesAPISettingKey); err == nil && strings.TrimSpace(v) != "" {
			if b, ok := parseBoolSetting(v); ok {
				return b
			}
		}
	}
	return p.responsesAPI
}
