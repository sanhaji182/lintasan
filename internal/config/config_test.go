package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Ensure env is clean for default test
	os.Unsetenv("PORT")
	os.Unsetenv("LINTASAN_DATA_DIR")
	os.Unsetenv("LINTASAN_MASTER_KEY")
	os.Unsetenv("MITM_PORT")
	os.Unsetenv("LINTASAN_MITM_ENABLED")
	os.Unsetenv("LINTASAN_OAUTH_IDE_ENABLED")
	os.Unsetenv("LINTASAN_OAUTH_PUBLIC_BASE_URL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Port != 20180 {
		t.Errorf("expected default port 20180, got %d", cfg.Port)
	}
	if cfg.MITMPort != 8443 {
		t.Errorf("expected default MITM port 8443, got %d", cfg.MITMPort)
	}
	if cfg.MITMEnabled != false {
		t.Errorf("expected MITM disabled by default, got %v", cfg.MITMEnabled)
	}
	if cfg.OAuthIDEEnabled != false {
		t.Errorf("expected OAuthIDE disabled by default, got %v", cfg.OAuthIDEEnabled)
	}
	if cfg.DBPath == "" {
		t.Error("DBPath must not be empty")
	}
}

func TestLoadCustomEnv(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("LINTASAN_DATA_DIR", "/tmp/lintasan-go-test")
	os.Setenv("LINTASAN_MASTER_KEY", "test-key-123")
	os.Setenv("MITM_PORT", "9443")
	os.Setenv("LINTASAN_MITM_ENABLED", "true")
	os.Setenv("LINTASAN_OAUTH_IDE_ENABLED", "1")
	os.Setenv("LINTASAN_OAUTH_PUBLIC_BASE_URL", "https://example.com/")
	defer os.RemoveAll("/tmp/lintasan-go-test")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("LINTASAN_DATA_DIR")
		os.Unsetenv("LINTASAN_MASTER_KEY")
		os.Unsetenv("MITM_PORT")
		os.Unsetenv("LINTASAN_MITM_ENABLED")
		os.Unsetenv("LINTASAN_OAUTH_IDE_ENABLED")
		os.Unsetenv("LINTASAN_OAUTH_PUBLIC_BASE_URL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DataDir != "/tmp/lintasan-go-test" {
		t.Errorf("expected data dir /tmp/lintasan-go-test, got %s", cfg.DataDir)
	}
	if cfg.MasterKey != "test-key-123" {
		t.Errorf("expected master key test-key-123, got %s", cfg.MasterKey)
	}
	if cfg.MITMPort != 9443 {
		t.Errorf("expected MITM port 9443, got %d", cfg.MITMPort)
	}
	if cfg.MITMEnabled != true {
		t.Errorf("expected MITM enabled, got %v", cfg.MITMEnabled)
	}
	if cfg.OAuthIDEEnabled != true {
		t.Errorf("expected OAuthIDE enabled, got %v", cfg.OAuthIDEEnabled)
	}
	if cfg.OAuthPublicBaseURL != "https://example.com" {
		t.Errorf("expected OAuthPublicBaseURL 'https://example.com', got '%s'", cfg.OAuthPublicBaseURL)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		fallback bool
		expected bool
	}{
		{"", "true", false, true},
		{"", "1", false, true},
		{"", "yes", false, true},
		{"", "on", false, true},
		{"", "false", true, false},
		{"", "0", true, false},
		{"", "no", true, false},
		{"", "off", true, false},
		{"", "", true, true},   // empty → fallback
		{"", "", false, false}, // empty → fallback
		{"", "invalid", true, true},   // unrecognized → fallback
		{"", "invalid", false, false}, // unrecognized → fallback
	}

	for _, tt := range tests {
		if tt.value != "" {
			os.Setenv("TEST_GETENV_BOOL", tt.value)
		} else {
			os.Unsetenv("TEST_GETENV_BOOL")
		}
		got := getEnvBool("TEST_GETENV_BOOL", tt.fallback)
		if got != tt.expected {
			t.Errorf("getEnvBool(TEST_GETENV_BOOL, %v) = %v when value=%q; want %v",
				tt.fallback, got, tt.value, tt.expected)
		}
		os.Unsetenv("TEST_GETENV_BOOL")
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		value    string
		fallback int
		expected int
	}{
		{"8080", 20180, 8080},
		{"", 20180, 20180},
		{"not-a-number", 42, 42},
		{"-1", 0, -1},
	}

	for _, tt := range tests {
		if tt.value != "" {
			os.Setenv("TEST_GETENV_INT", tt.value)
		} else {
			os.Unsetenv("TEST_GETENV_INT")
		}
		got := getEnvInt("TEST_GETENV_INT", tt.fallback)
		if got != tt.expected {
			t.Errorf("getEnvInt(TEST_GETENV_INT, %d) = %d when value=%q; want %d",
				tt.fallback, got, tt.value, tt.expected)
		}
		os.Unsetenv("TEST_GETENV_INT")
	}
}
