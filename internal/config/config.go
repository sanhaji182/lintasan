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
}

func Load() (*Config, error) {
	dataDir := getEnv("LINTASAN_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	return &Config{
		Port:      getEnvInt("PORT", 20180),
		DBPath:    filepath.Join(dataDir, "lintasan.db"),
		DataDir:   dataDir,
		MasterKey:   getEnv("LINTASAN_MASTER_KEY", ""),
		MITMPort:    getEnvInt("MITM_PORT", 8443),
		MITMEnabled: getEnvBool("LINTASAN_MITM_ENABLED", false),
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
