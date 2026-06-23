package config

import (
	"testing"
	"time"
)

func TestDevModeOverridesRuntimeValues(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("POLZA_API_KEY", "")
	t.Setenv("POLZA_SILENT_ON_MISSING", "true")
	t.Setenv("BOT_DEV_MODE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Bot.Chattiness != "high" {
		t.Fatalf("expected high chattiness, got %s", cfg.Bot.Chattiness)
	}
	if cfg.Bot.DirectCooldown != 10*time.Second {
		t.Fatalf("expected 10s direct cooldown, got %s", cfg.Bot.DirectCooldown)
	}
	if cfg.Bot.AmbientCooldown != 20*time.Second {
		t.Fatalf("expected 20s ambient cooldown, got %s", cfg.Bot.AmbientCooldown)
	}
	if cfg.Bot.MaxRepliesPerHour != 60 {
		t.Fatalf("expected 60 hourly replies, got %d", cfg.Bot.MaxRepliesPerHour)
	}
}
