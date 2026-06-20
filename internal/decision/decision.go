package decision

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/storage"
	"github.com/w6itec6apel/gofer/internal/telegram"
)

type EventType string

const (
	EventDirectMention EventType = "DIRECT_MENTION"
	EventReplyToBot    EventType = "REPLY_TO_BOT"
	EventCommand       EventType = "COMMAND"
	EventNameMention   EventType = "NAME_MENTION"
	EventQuestion      EventType = "QUESTION"
	EventTechTopic     EventType = "TECH_TOPIC"
	EventHumorTrigger  EventType = "HUMOR_TRIGGER"
	EventIdleProactive EventType = "IDLE_PROACTIVE"
	EventSmallTalk     EventType = "SMALL_TALK"
)

type Event struct {
	Type      EventType
	Text      string
	ChatID    int64
	UserID    int64
	MessageID int
	IsCommand bool
}

type Decision struct {
	Respond     bool
	Reason      string
	Event       Event
	Probability float64
}

type Engine struct {
	cfg   config.BotConfig
	store *storage.Store
}

func NewEngine(cfg config.BotConfig, store *storage.Store) *Engine {
	return &Engine{cfg: cfg, store: store}
}

func (e *Engine) Decide(message telegram.Message, settings storage.ChatSettings) Decision {
	event := e.Classify(message)
	if event.Text == "" {
		return Decision{Respond: false, Reason: "empty_text", Event: event}
	}
	if event.Type == EventCommand {
		return Decision{Respond: true, Reason: "command", Event: event, Probability: 1}
	}
	if !settings.Enabled {
		return Decision{Respond: false, Reason: "bot_disabled", Event: event}
	}
	if settings.SilentUntil.After(time.Now()) && event.Type != EventDirectMention && event.Type != EventReplyToBot && event.Type != EventNameMention {
		return Decision{Respond: false, Reason: "silent_mode", Event: event}
	}

	if event.Type == EventDirectMention || event.Type == EventReplyToBot || event.Type == EventNameMention {
		return Decision{Respond: true, Reason: "direct_trigger", Event: event, Probability: 1}
	}

	allowed, reason := e.withinLimits(message.Chat.ID, settings)
	if !allowed {
		return Decision{Respond: false, Reason: reason, Event: event}
	}

	probability := e.probability(event)
	if rand.Float64() <= probability {
		return Decision{Respond: true, Reason: "probability_passed", Event: event, Probability: probability}
	}
	return Decision{Respond: false, Reason: "probability_skipped", Event: event, Probability: probability}
}

func (e *Engine) Classify(message telegram.Message) Event {
	text := strings.TrimSpace(message.Text)
	event := Event{
		Type:      EventSmallTalk,
		Text:      text,
		ChatID:    message.Chat.ID,
		MessageID: message.MessageID,
	}
	if message.From != nil {
		event.UserID = message.From.ID
	}

	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "/") {
		event.Type = EventCommand
		event.IsCommand = true
		return event
	}
	if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil && message.ReplyToMessage.From.IsBot {
		event.Type = EventReplyToBot
		return event
	}
	if e.cfg.Username != "" && strings.Contains(lower, "@"+strings.ToLower(e.cfg.Username)) {
		event.Type = EventDirectMention
		return event
	}
	for _, trigger := range e.cfg.NameTriggers {
		if strings.Contains(lower, strings.ToLower(trigger)) {
			event.Type = EventNameMention
			return event
		}
	}
	if strings.Contains(text, "?") || strings.HasPrefix(lower, "как ") || strings.HasPrefix(lower, "почему ") || strings.HasPrefix(lower, "что ") {
		event.Type = EventQuestion
		return event
	}
	if isGoTopic(lower) {
		event.Type = EventTechTopic
		return event
	}
	if containsAny(lower, []string{"баг", "bug", "деплой", "deploy", "сервер", "server", "prod", "docker", "k8s", "api", "panic"}) {
		event.Type = EventTechTopic
		return event
	}
	if containsAny(lower, []string{"лол", "ха", "мем", "классика", "опять", "магия"}) {
		event.Type = EventHumorTrigger
		return event
	}
	return event
}

func (e *Engine) DecideProactive(chatID int64, settings storage.ChatSettings) Decision {
	event := Event{Type: EventIdleProactive, ChatID: chatID, Text: "Напиши короткое инициативное сообщение по недавнему контексту чата."}
	if !settings.Enabled || !settings.ProactiveEnabled {
		return Decision{Respond: false, Reason: "proactive_disabled", Event: event}
	}
	if settings.SilentUntil.After(time.Now()) {
		return Decision{Respond: false, Reason: "silent_mode", Event: event}
	}

	lastMessage := e.store.LastMessageAt(chatID)
	if lastMessage.IsZero() || time.Since(lastMessage) < e.cfg.ProactiveIdleAfter {
		return Decision{Respond: false, Reason: "chat_not_idle", Event: event}
	}
	allowed, reason := e.withinLimits(chatID, settings)
	if !allowed {
		return Decision{Respond: false, Reason: reason, Event: event}
	}
	dayStart := beginningOfDay(time.Now())
	if e.store.CountProactiveSince(chatID, dayStart) >= settings.MaxProactivePerDay {
		return Decision{Respond: false, Reason: "daily_proactive_limit", Event: event}
	}

	probability := e.cfg.ResponseProbability.ProactiveMin
	if e.cfg.ResponseProbability.ProactiveMax > probability {
		probability += rand.Float64() * (e.cfg.ResponseProbability.ProactiveMax - probability)
	}
	if rand.Float64() <= probability {
		return Decision{Respond: true, Reason: "idle_probability_passed", Event: event, Probability: probability}
	}
	return Decision{Respond: false, Reason: "idle_probability_skipped", Event: event, Probability: probability}
}

func (e *Engine) withinLimits(chatID int64, settings storage.ChatSettings) (bool, string) {
	lastBot := e.store.LastBotMessageAt(chatID)
	minDelay := time.Duration(settings.MinDelaySeconds) * time.Second
	if !lastBot.IsZero() && time.Since(lastBot) < minDelay {
		return false, "cooldown"
	}
	if e.store.CountBotMessagesSince(chatID, time.Now().Add(-time.Hour)) >= settings.MaxRepliesPerHour {
		return false, "hourly_reply_limit"
	}
	if e.store.TokenUsageSince(chatID, beginningOfDay(time.Now())) >= settings.DailyTokenLimit {
		return false, "daily_token_limit"
	}
	return true, ""
}

func (e *Engine) probability(event Event) float64 {
	switch event.Type {
	case EventQuestion:
		return e.cfg.ResponseProbability.Question
	case EventTechTopic:
		if isGoTopic(event.Text) {
			return e.cfg.ResponseProbability.GoTopic
		}
		return e.cfg.ResponseProbability.TechTopic
	case EventHumorTrigger:
		return e.cfg.ResponseProbability.HumorTrigger
	case EventSmallTalk:
		return e.cfg.ResponseProbability.SmallTalk
	default:
		return 0
	}
}

func isGoTopic(text string) bool {
	return containsAny(strings.ToLower(text), []string{"go", "golang", "goroutine", "channel", "context", "interface", "дженерик"})
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func beginningOfDay(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
