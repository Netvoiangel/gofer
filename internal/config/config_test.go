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
	if cfg.Bot.Chattiness != "insane" {
		t.Fatalf("expected insane chattiness, got %s", cfg.Bot.Chattiness)
	}
	if cfg.Bot.DirectCooldown != 3*time.Second {
		t.Fatalf("expected 3s direct cooldown, got %s", cfg.Bot.DirectCooldown)
	}
	if cfg.Bot.AmbientCooldown != 8*time.Second {
		t.Fatalf("expected 8s ambient cooldown, got %s", cfg.Bot.AmbientCooldown)
	}
	if cfg.Bot.MaxRepliesPerHour != 300 {
		t.Fatalf("expected 300 hourly replies, got %d", cfg.Bot.MaxRepliesPerHour)
	}
}
