package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port        int
	DBPath      string
	DataDir     string
	MasterKey   string
	MITMPort    int
	MITMEnabled bool
	// RateLimitPerMin sets the per-key rate limit (requests/minute).
	// 0 means the default (60) — not a true zero/unlimited sentinel because
	// a separate rlEnabled check disables the limiter at zero. Use the env
	// var LINTASAN_RATELIMIT_PERMIN at runtime for temporary overrides
	// without recompiling.
	RateLimitPerMin int
	// RateLimitBurst sets the burst window size. 0 means the default (30).
	// Override at runtime via LINTASAN_RATELIMIT_BURST.
	RateLimitBurst int
	// OAuthIDEEnabled gates the experimental IDE OAuth lab (default OFF).
	OAuthIDEEnabled bool
	// OAuthPublicBaseURL is the public origin for redirect_uri (e.g. https://lintasan.example.com).
	OAuthPublicBaseURL string
}

func Load() (*Config, error) {
	dataDir := getEnv("LINTASAN_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	return &Config{
		Port:               getEnvInt("PORT", 20180),
		DBPath:             filepath.Join(dataDir, "lintasan.db"),
		DataDir:            dataDir,
		MasterKey:          getEnv("LINTASAN_MASTER_KEY", ""),
		MITMPort:           getEnvInt("MITM_PORT", 8443),
		MITMEnabled:        getEnvBool("LINTASAN_MITM_ENABLED", false),
		RateLimitPerMin:    getEnvInt("LINTASAN_RATELIMIT_PERMIN", 0),
		RateLimitBurst:     getEnvInt("LINTASAN_RATELIMIT_BURST", 0),
		OAuthIDEEnabled:    getEnvBool("LINTASAN_OAUTH_IDE_ENABLED", false),
		OAuthPublicBaseURL: strings.TrimRight(getEnv("LINTASAN_OAUTH_PUBLIC_BASE_URL", ""), "/"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
