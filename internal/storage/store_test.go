package storage

import (
	"testing"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
)

func TestSettingsPersistAfterReload(t *testing.T) {
	path := t.TempDir() + "/state.json"
	defaults := config.BotConfig{
		DefaultMode:        "funny",
		MinDelay:          3 * time.Minute,
		MaxRepliesPerHour: 10,
		ContextLimit:      50,
		PrivacyStoreText:  true,
	}

	store, err := New(path, defaults)
	if err != nil {
		t.Fatalf("storage init: %v", err)
	}
	if err := store.UpdateSettings(10, func(settings *ChatSettings) {
		settings.Enabled = false
		settings.Mode = "calm"
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	reloaded, err := New(path, defaults)
	if err != nil {
		t.Fatalf("storage reload: %v", err)
	}
	settings := reloaded.Settings(10)
	if settings.Enabled {
		t.Fatalf("expected disabled setting to persist")
	}
	if settings.Mode != "calm" {
		t.Fatalf("expected calm mode, got %s", settings.Mode)
	}
}
