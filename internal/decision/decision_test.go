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

func TestDirectMentionRespondsWhenLimitsAllow(t *testing.T) {
	engine := newTestEngine(t)
	settings := testSettings(10)

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

func TestReplyToBotRespondsWhenLimitsAllow(t *testing.T) {
	engine := newTestEngine(t)
	settings := testSettings(10)

	decision := engine.Decide(telegram.Message{
		MessageID: 1,
		Text:      "а можно короче?",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
		ReplyToMessage: &telegram.Message{
			From: &telegram.User{ID: 30, IsBot: true},
		},
	}, settings)

	if !decision.Respond {
		t.Fatalf("expected reply-to-bot response, got reason %s", decision.Reason)
	}
	if decision.Event.Type != EventReplyToBot {
		t.Fatalf("expected reply-to-bot event, got %s", decision.Event.Type)
	}
}

func TestSoftDirectAfterRecentBotMessage(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	if err := store.AddMessage(storage.MessageRecord{
		Time:   time.Now().UTC(),
		ChatID: 10,
		IsBot:  true,
		Text:   "живой ответ",
	}, 50); err != nil {
		t.Fatalf("add bot message: %v", err)
	}

	event := engine.Classify(telegram.Message{
		MessageID: 2,
		Text:      "Ты будешь отвечать?",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	})

	if event.Type != EventSoftDirect {
		t.Fatalf("expected soft direct, got %s", event.Type)
	}
}

func TestSoftDirectChatPhrasesFromLiveChat(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	if err := store.AddMessage(storage.MessageRecord{
		Time:   time.Now().UTC(),
		ChatID: 10,
		IsBot:  true,
		Text:   "предыдущая реплика",
	}, 50); err != nil {
		t.Fatalf("add bot message: %v", err)
	}

	for _, text := range []string{"бля че с ним", "он не хочет отвечать", "не молчи"} {
		event := engine.Classify(telegram.Message{
			MessageID: 2,
			Text:      text,
			Chat:      telegram.Chat{ID: 10},
			From:      &telegram.User{ID: 20},
			Date:      time.Now().Unix(),
		})
		if event.Type != EventSoftDirect {
			t.Fatalf("expected soft direct for %q, got %s", text, event.Type)
		}
	}
}

func TestSoftDirectUsesDirectCooldown(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventDirectMention),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	decision := engine.DecideEvent(Event{
		Type:   EventSoftDirect,
		Text:   "Ты будешь отвечать?",
		ChatID: 10,
		UserID: 20,
	}, settings)

	if decision.Respond {
		t.Fatalf("expected soft direct cooldown")
	}
	if decision.CooldownChannel != "direct" {
		t.Fatalf("expected direct cooldown channel, got %s", decision.CooldownChannel)
	}
}

func TestCooldownLimitsDirectMention(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventDirectMention),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	decision := engine.Decide(telegram.Message{
		MessageID: 2,
		Text:      "@gopher_bot ping",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if decision.Respond {
		t.Fatalf("expected cooldown skip")
	}
	if decision.Reason != "cooldown" {
		t.Fatalf("expected cooldown reason, got %s", decision.Reason)
	}
}

func TestHourlyLimitSkipsResponse(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	settings.MinDelaySeconds = 0
	settings.MaxRepliesPerHour = 1
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().Add(-10 * time.Minute).UTC(),
		ChatID:    10,
		EventType: string(EventTechTopic),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	decision := engine.Decide(telegram.Message{
		MessageID: 2,
		Text:      "@gopher_bot ping",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if decision.Respond {
		t.Fatalf("expected hourly limit skip")
	}
	if decision.Reason != "hourly_reply_limit" {
		t.Fatalf("expected hourly limit reason, got %s", decision.Reason)
	}
}

func TestDisabledBotSkipsNonCommand(t *testing.T) {
	engine := newTestEngine(t)
	settings := testSettings(10)
	settings.Enabled = false

	decision := engine.Decide(telegram.Message{
		MessageID: 1,
		Text:      "@gopher_bot ping",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if decision.Respond {
		t.Fatalf("expected disabled bot to skip")
	}
	if decision.Reason != "bot_disabled" {
		t.Fatalf("expected bot_disabled reason, got %s", decision.Reason)
	}
}

func TestCommandRespondsWhenBotDisabled(t *testing.T) {
	engine := newTestEngine(t)
	settings := testSettings(10)
	settings.Enabled = false

	decision := engine.Decide(telegram.Message{
		MessageID: 1,
		Text:      "/gopher_on",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if !decision.Respond {
		t.Fatalf("expected command response, got reason %s", decision.Reason)
	}
	if !decision.Event.IsCommand {
		t.Fatalf("expected command event")
	}
}

func TestCommandDoesNotBlockDirectMentionCooldown(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventCommand),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	decision := engine.Decide(telegram.Message{
		MessageID: 2,
		Text:      "@gopher_bot ping",
		Chat:      telegram.Chat{ID: 10},
		From:      &telegram.User{ID: 20},
		Date:      time.Now().Unix(),
	}, settings)

	if !decision.Respond {
		t.Fatalf("expected command not to block direct mention, got reason %s", decision.Reason)
	}
}

func TestLocalReactionCooldown(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventLocalReaction),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	allowed, reason := engine.CanLocalReact(10, settings)
	if allowed {
		t.Fatalf("expected local cooldown")
	}
	if reason != "local_cooldown" {
		t.Fatalf("expected local_cooldown reason, got %s", reason)
	}
}

func TestCommandDoesNotBlockLocalReaction(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventCommand),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	allowed, reason := engine.CanLocalReact(10, settings)
	if !allowed {
		t.Fatalf("expected command not to block local reaction, got %s", reason)
	}
}

func TestAmbientCooldownDoesNotBlockLocalReaction(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventTechTopic),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	allowed, reason := engine.CanLocalReact(10, settings)
	if !allowed {
		t.Fatalf("expected ambient cooldown not to block local reaction, got %s", reason)
	}
}

func TestTechTopicCooldownCanBeQueued(t *testing.T) {
	engine, store := newTestEngineWithStore(t)
	settings := testSettings(10)
	if err := store.LogEvent(storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    10,
		EventType: string(EventTechTopic),
		Answered:  true,
	}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	decision := engine.DecideEvent(Event{
		Type:   EventTechTopic,
		Text:   "сломался сервер",
		ChatID: 10,
		UserID: 20,
	}, settings)

	if decision.Respond {
		t.Fatalf("expected ambient cooldown")
	}
	if decision.Reason != "cooldown" {
		t.Fatalf("expected cooldown reason, got %s", decision.Reason)
	}
	if decision.CooldownChannel != "ambient_llm" {
		t.Fatalf("expected ambient_llm channel, got %s", decision.CooldownChannel)
	}
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	engine, _ := newTestEngineWithStore(t)
	return engine
}

func newTestEngineWithStore(t *testing.T) (*Engine, *storage.Store) {
	t.Helper()

	botCfg := config.BotConfig{
		Username:           "gopher_bot",
		NameTriggers:       []string{"гофер", "gopher", "бот"},
		DefaultMode:        "funny",
		MinDelay:           3 * time.Minute,
		CommandCooldown:    3 * time.Second,
		DirectCooldown:     15 * time.Second,
		AmbientCooldown:    45 * time.Second,
		LocalCooldown:      20 * time.Second,
		ProactiveCooldown:  20 * time.Minute,
		SoftDirectWindow:   5 * time.Minute,
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
	return NewEngine(botCfg, store), store
}

func testSettings(chatID int64) storage.ChatSettings {
	return storage.ChatSettings{
		ChatID:             chatID,
		Enabled:            true,
		Mode:               "funny",
		ProactiveEnabled:   true,
		MinDelaySeconds:    180,
		MaxRepliesPerHour:  10,
		MaxProactivePerDay: 5,
		DailyTokenLimit:    1000,
	}
}
