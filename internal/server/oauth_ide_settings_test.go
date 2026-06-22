package server

import (
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestOAuthIdeEnabledDBOverridesEnv(t *testing.T) {
	cfg := &config.Config{OAuthIDEEnabled: true}
	s, _ := newTestServer(t, cfg)
	if err := s.db.SetSetting("oauth_ide_enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if s.oauthIdeEnabled() {
		t.Fatal("DB false should override env true")
	}
	if err := s.db.SetSetting("oauth_ide_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if !s.oauthIdeEnabled() {
		t.Fatal("DB true should enable")
	}
}

func TestOAuthIdeEnabledEnvWhenDBUnset(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: false})
	if s.oauthIdeEnabled() {
		t.Fatal("env false, no DB key")
	}
	s.cfg.OAuthIDEEnabled = true
	if !s.oauthIdeEnabled() {
		t.Fatal("env true when DB unset")
	}
}