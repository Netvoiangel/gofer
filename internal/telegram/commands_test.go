package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/storage"
)

func TestBudgetShowsCooldownChannels(t *testing.T) {
	botCfg := config.BotConfig{
		DefaultMode:        "angry",
		Chattiness:         "high",
		ProfanityLevel:     "medium",
		MinDelay:           35 * time.Second,
		CommandCooldown:    3 * time.Second,
		DirectCooldown:     15 * time.Second,
		AmbientCooldown:    45 * time.Second,
		LocalCooldown:      20 * time.Second,
		ProactiveCooldown:  1200 * time.Second,
		Debounce:           8 * time.Second,
		MaxRepliesPerHour:  40,
		MaxProactivePerDay: 8,
		DailyTokenLimit:    20000,
		PrivacyStoreText:   true,
	}
	store, err := storage.New(t.TempDir()+"/state.json", botCfg)
	if err != nil {
		t.Fatalf("storage init: %v", err)
	}
	handler := NewCommandHandler(nil, store, botCfg, nil)

	text, handled := handler.Handle(context.Background(), Message{
		Text: "/gopher_budget",
		Chat: Chat{ID: 10},
		From: &User{ID: 20},
	})
	if !handled {
		t.Fatalf("expected command handled")
	}

	for _, want := range []string{
		"Разговорчивость: high",
		"Мат: medium",
		"- command: 3 сек",
		"- direct/reply: 15 сек",
		"- ambient LLM: 45 сек",
		"- local reaction: 20 сек",
		"- proactive: 1200 сек",
		"- debounce: 8 сек",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("budget output missing %q:\n%s", want, text)
		}
	}
}
