package compress

import (
	"strings"
)

// CompressionMode defines the compression level.
type CompressionMode string

const (
	ModeOff        CompressionMode = "off"
	ModeLite       CompressionMode = "lite"
	ModeStandard   CompressionMode = "standard"
	ModeAggressive CompressionMode = "aggressive"
)

// PipelineStats tracks compression savings.
type PipelineStats struct {
	Mode            string  `json:"mode"`
	OriginalBytes   int     `json:"original_bytes"`
	CompressedBytes int     `json:"compressed_bytes"`
	SavingsBytes    int     `json:"savings_bytes"`
	SavingsPercent  float64 `json:"savings_percent"`
	RTKApplied      bool    `json:"rtk_applied"`
	CavemanApplied  bool    `json:"caveman_applied"`
	ItemsCompressed int     `json:"items_compressed"`
}

// CompressPipeline runs stacked compression: RTK on tool outputs, Caveman on system prompt.
// Mode controls intensity: off, lite, standard, aggressive.
func CompressPipeline(content string, mode string) (string, PipelineStats) {
	stats := PipelineStats{
		Mode:          mode,
		OriginalBytes: len(content),
	}

	if mode == "off" || mode == "" {
		stats.CompressedBytes = len(content)
		return content, stats
	}

	compressed := content

	// RTK compression on tool outputs (all modes except off)
	if mode != "off" {
		result, savings := CompressRTK(compressed)
		if savings > 0 {
			compressed = result
			stats.RTKApplied = true
			stats.ItemsCompressed++
			stats.SavingsPercent = savings
		}
	}

	// Generic compression for all modes
	if mode == "aggressive" || mode == "standard" {
		result := CompressGeneric(compressed)
		if len(result) < len(compressed) {
			compressed = result
		}
	}

	stats.CompressedBytes = len(compressed)
	stats.SavingsBytes = stats.OriginalBytes - stats.CompressedBytes
	if stats.OriginalBytes > 0 {
		stats.SavingsPercent = float64(stats.SavingsBytes) / float64(stats.OriginalBytes)
	}

	return compressed, stats
}

// CompressMessages applies compression to message content in a chat request.
// Scans for tool_result and user messages with large content, compresses them.
func CompressMessages(messages []map[string]any, mode string) ([]map[string]any, PipelineStats) {
	if mode == "off" || mode == "" {
		return messages, PipelineStats{Mode: mode}
	}

	var totalOrig, totalComp int
	var itemsCompressed int

	result := make([]map[string]any, len(messages))
	copy(result, messages)

	for i, msg := range result {
		role, _ := msg["role"].(string)

		// Only compress tool results and user messages with large content
		if role != "tool" && role != "user" {
			continue
		}

		content := getContentFromMsg(msg)
		if len(content) < 200 {
			continue
		}

		compressed, savings := CompressRTK(content)
		if savings > 0.1 { // Only apply if >10% savings
			totalOrig += len(content)
			totalComp += len(compressed)
			itemsCompressed++

			// Update message content
			newMsg := make(map[string]any)
			for k, v := range msg {
				newMsg[k] = v
			}
			newMsg["content"] = compressed
			result[i] = newMsg
		}
	}

	stats := PipelineStats{
		Mode:            mode,
		OriginalBytes:   totalOrig,
		CompressedBytes: totalComp,
		SavingsBytes:    totalOrig - totalComp,
		RTKApplied:      itemsCompressed > 0,
		ItemsCompressed: itemsCompressed,
	}
	if totalOrig > 0 {
		stats.SavingsPercent = float64(totalOrig-totalComp) / float64(totalOrig)
	}

	// Inject caveman prompt for standard/aggressive
	if mode == "standard" || mode == "aggressive" {
		cavemanMode := "standard"
		if mode == "aggressive" {
			cavemanMode = "aggressive"
		}
		result = InjectCaveman(result, cavemanMode)
		stats.CavemanApplied = true
	}

	return result, stats
}

// getContentFromMsg extracts content string from a message.
func getContentFromMsg(msg map[string]any) string {
	switch v := msg["content"].(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// ParseMode validates and returns a CompressionMode.
func ParseMode(mode string) CompressionMode {
	switch strings.ToLower(mode) {
	case "off":
		return ModeOff
	case "lite":
		return ModeLite
	case "standard":
		return ModeStandard
	case "aggressive":
		return ModeAggressive
	default:
		return ModeStandard
	}
}
