package decision

import (
	"testing"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/storage"
	"github.com/w6itec6apel/gofer/internal/telegram"
)

func TestClassifyDirectMention(t *testing.T) {
	engine := newTestEngine(t)
	event := engine.Classify(telegram.Message{
		MessageID: 1,
		Text:      "@gopher_bot объясни context",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
	})

	if event.Type != EventDirectMention {
		t.Fatalf("expected direct mention, got %s", event.Type)
	}
}

func TestClassifyQuestion(t *testing.T) {
	engine := newTestEngine(t)
	event := engine.Classify(telegram.Message{
		MessageID: 1,
		Text:      "почему Go так любит явные ошибки?",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
	})

	if event.Type != EventQuestion {
		t.Fatalf("expected question, got %s", event.Type)
	}
}

func TestDirectMentionBypassesCooldown(t *testing.T) {
	engine := newTestEngine(t)
	settings := storage.ChatSettings{
		ChatID:             10,
		Enabled:            true,
		Mode:               "funny",
		ProactiveEnabled:   true,
		MinDelaySeconds:    180,
		MaxRepliesPerHour:  1,
		MaxProactivePerDay: 5,
		DailyTokenLimit:    1000,
	}

	decision := engine.Decide(telegram.Message{
		MessageID: 1,
		Text:      "@gopher_bot ping",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if !decision.Respond {
		t.Fatalf("expected direct mention response, got reason %s", decision.Reason)
	}
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()

	botCfg := config.BotConfig{
		Username:           "gopher_bot",
		NameTriggers:       []string{"гофер", "gopher", "бот"},
		DefaultMode:        "funny",
		MinDelay:           3 * time.Minute,
		MaxRepliesPerHour:  10,
		MaxProactivePerDay: 5,
		DailyTokenLimit:    20000,
		ContextLimit:       50,
		ResponseProbability: config.ProbabilityConfig{
			Question:     0.40,
			GoTopic:      0.50,
			TechTopic:    0.25,
			HumorTrigger: 0.10,
			SmallTalk:    0.02,
			ProactiveMin: 0.05,
			ProactiveMax: 0.15,
		},
	}
	store, err := storage.New(t.TempDir()+"/state.json", botCfg)
	if err != nil {
		t.Fatalf("storage init: %v", err)
	}
	return NewEngine(botCfg, store)
}
