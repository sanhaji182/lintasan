package server

import (
	"fmt"
	"strings"
)

const (
	fallbackViewSetting  = "fallback_chains"
	fallbackModelSetting = "fallback_model_chains"
	fallbackConnSetting  = "fallback_connection_chains"
)

// syncFallbackEngine translates the dashboard's named chain list into the
// primary -> fallbacks map shape consumed by internal/fallback, then reloads
// the engine. The dashboard stores [primary, fallback...] per named chain.
//
// force=false skips writing when the dashboard list has no usable chains, so
// boot-time migration never wipes maps written by another tool.
func (s *Server) syncFallbackEngine(force bool) {
	data := asMap(s.getJSONSetting(fallbackViewSetting, map[string]any{}))
	modelChains := map[string][]string{}
	connChains := map[string][]string{}
	collect := func(key string, dst map[string][]string) int {
		chains := 0
		for _, item := range asSlice(data[key]) {
			raw := asSlice(asMap(item)["chain"])
			chain := make([]string, 0, len(raw))
			for _, value := range raw {
				id := strings.TrimSpace(fmt.Sprint(value))
				if id != "" {
					chain = append(chain, id)
				}
			}
			if len(chain) < 2 {
				continue
			}
			dst[chain[0]] = chain[1:]
			chains++
		}
		return chains
	}
	chains := collect("model_chains", modelChains) + collect("connection_chains", connChains)
	if chains == 0 && !force {
		return
	}
	s.setJSONSetting(fallbackModelSetting, modelChains)
	s.setJSONSetting(fallbackConnSetting, connChains)
	if s.proxy != nil {
		s.proxy.ReloadFallbackChains()
	}
}
