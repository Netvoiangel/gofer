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
	EventSoftDirect    EventType = "SOFT_DIRECT"
	EventCommand       EventType = "COMMAND"
	EventNameMention   EventType = "NAME_MENTION"
	EventQuestion      EventType = "QUESTION"
	EventTechTopic     EventType = "TECH_TOPIC"
	EventHumorTrigger  EventType = "HUMOR_TRIGGER"
	EventIdleProactive EventType = "IDLE_PROACTIVE"
	EventLocalReaction EventType = "LOCAL_REACTION"
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
	Respond          bool
	Reason           string
	Event            Event
	Probability      float64
	Roll             float64
	CooldownChannel  string
	RemainingSeconds int
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
	return e.DecideEvent(event, settings)
}

func (e *Engine) DecideEvent(event Event, settings storage.ChatSettings) Decision {
	if event.Text == "" {
		return Decision{Respond: false, Reason: "empty_text", Event: event}
	}
	if event.Type == EventCommand {
		allowed, reason := e.withinCommandLimits(event)
		if !allowed {
			return Decision{Respond: false, Reason: reason, Event: event, CooldownChannel: "command"}
		}
		return Decision{Respond: true, Reason: "command", Event: event, Probability: 1, CooldownChannel: "command"}
	}
	if !settings.Enabled {
		return Decision{Respond: false, Reason: "bot_disabled", Event: event}
	}
	if isBotPushbackText(event.Text) {
		return Decision{Respond: false, Reason: "bot_pushback", Event: event, CooldownChannel: e.cooldownChannel(event.Type)}
	}
	allowed, reason := e.withinLimits(event, settings)
	if !allowed {
		return Decision{Respond: false, Reason: reason, Event: event, CooldownChannel: e.cooldownChannel(event.Type), RemainingSeconds: e.remainingCooldown(event, settings)}
	}

	if event.Type == EventDirectMention || event.Type == EventReplyToBot || event.Type == EventNameMention || event.Type == EventSoftDirect {
		return Decision{Respond: true, Reason: "direct_trigger", Event: event, Probability: 1, CooldownChannel: e.cooldownChannel(event.Type)}
	}

	probability := e.probability(event)
	roll := rand.Float64()
	if roll <= probability {
		return Decision{Respond: true, Reason: "probability_passed", Event: event, Probability: probability, Roll: roll, CooldownChannel: e.cooldownChannel(event.Type)}
	}
	return Decision{Respond: false, Reason: "probability_skipped", Event: event, Probability: probability, Roll: roll, CooldownChannel: e.cooldownChannel(event.Type)}
}

func (e *Engine) CanLocalReact(chatID int64, settings storage.ChatSettings) (bool, string) {
	if !settings.Enabled {
		return false, "bot_disabled"
	}
	if e.store.CountAnsweredEventsSince(chatID, time.Now().Add(-time.Hour), string(EventCommand)) >= settings.MaxRepliesPerHour {
		return false, "hourly_reply_limit"
	}
	last := e.store.LastAnsweredEventAt(chatID, string(EventLocalReaction))
	if !last.IsZero() && time.Since(last) < e.cfg.LocalCooldown {
		return false, "local_cooldown"
	}
	return true, ""
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
	if e.isSoftDirect(message.Chat.ID, lower) {
		event.Type = EventSoftDirect
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

func (e *Engine) isSoftDirect(chatID int64, lowerText string) bool {
	lastBot := e.store.LastBotMessageAt(chatID)
	if lastBot.IsZero() || time.Since(lastBot) > e.cfg.SoftDirectWindow {
		return false
	}
	triggers := []string{
		"ты будешь отвечать",
		"ты тут",
		"ты живой",
		"ответь",
		"ответь мне",
		"че молчишь",
		"чё молчишь",
		"че с ним",
		"чё с ним",
		"что с ним",
		"не хочет отвечать",
		"не хочет",
		"не молчи",
		"бот молчит",
		"гофер молчит",
		"алло",
		"проснись",
	}
	return containsAny(lowerText, triggers)
}

func (e *Engine) DecideProactive(chatID int64, settings storage.ChatSettings) Decision {
	event := Event{Type: EventIdleProactive, ChatID: chatID, Text: "Напиши одну короткую злую реплику по свежему контексту чата. Не начинай со слов 'слушайте', 'давайте', 'соберёмся'."}
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
	if e.recentBotPushback(chatID) {
		return Decision{Respond: false, Reason: "recent_pushback", Event: event}
	}
	allowed, reason := e.withinProactiveLimits(chatID, settings)
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

func (e *Engine) recentBotPushback(chatID int64) bool {
	messages := e.store.RecentMessages(chatID, 8)
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.IsBot {
			continue
		}
		text := strings.ToLower(message.Text)
		if isBotPushbackText(text) || containsAny(text, []string{
			"гофер стал",
			"он заебал",
			"бесит",
			"уебище",
			"дегенерат",
			"дебил",
		}) {
			return true
		}
	}
	return false
}

func isBotPushbackText(text string) bool {
	return containsAny(strings.ToLower(text), []string{
		"гофер ебнулся",
		"гофер ебанулся",
		"бот ебнулся",
		"бот ебанулся",
		"он ебнулся",
		"он ебанулся",
		"ты заебал",
		"заебал пиздеть",
		"иди нахуй",
		"пошел нахуй",
		"пошёл нахуй",
		"заткнись",
		"не пиши",
		"не отвлекай",
		"не выебывайся",
		"сдохни",
	})
}

func (e *Engine) withinLimits(event Event, settings storage.ChatSettings) (bool, string) {
	if e.store.CountAnsweredEventsSince(event.ChatID, time.Now().Add(-time.Hour), string(EventCommand)) >= settings.MaxRepliesPerHour {
		return false, "hourly_reply_limit"
	}
	if e.store.TokenUsageSince(event.ChatID, beginningOfDay(time.Now())) >= settings.DailyTokenLimit {
		return false, "daily_token_limit"
	}
	cooldown := e.cooldownFor(event.Type, settings)
	if cooldown <= 0 {
		return true, ""
	}
	last := e.store.LastAnsweredEventAt(event.ChatID, cooldownEventTypes(event.Type)...)
	if !last.IsZero() && time.Since(last) < cooldown {
		return false, "cooldown"
	}
	return true, ""
}

func (e *Engine) withinCommandLimits(event Event) (bool, string) {
	last := e.store.LastAnsweredEventAt(event.ChatID, string(EventCommand))
	if !last.IsZero() && time.Since(last) < e.cfg.CommandCooldown {
		return false, "command_cooldown"
	}
	return true, ""
}

func (e *Engine) withinProactiveLimits(chatID int64, settings storage.ChatSettings) (bool, string) {
	if e.store.CountAnsweredEventsSince(chatID, time.Now().Add(-time.Hour), string(EventCommand)) >= settings.MaxRepliesPerHour {
		return false, "hourly_reply_limit"
	}
	if e.store.TokenUsageSince(chatID, beginningOfDay(time.Now())) >= settings.DailyTokenLimit {
		return false, "daily_token_limit"
	}
	last := e.store.LastAnsweredEventAt(chatID, string(EventIdleProactive))
	if !last.IsZero() && time.Since(last) < e.cfg.ProactiveCooldown {
		return false, "proactive_cooldown"
	}
	return true, ""
}

func (e *Engine) cooldownFor(eventType EventType, settings storage.ChatSettings) time.Duration {
	switch eventType {
	case EventDirectMention, EventReplyToBot, EventNameMention, EventSoftDirect:
		return e.cfg.DirectCooldown
	default:
		if e.cfg.AmbientCooldown > 0 {
			return e.cfg.AmbientCooldown
		}
		return time.Duration(settings.MinDelaySeconds) * time.Second
	}
}

func cooldownEventTypes(eventType EventType) []string {
	switch eventType {
	case EventDirectMention, EventReplyToBot, EventNameMention, EventSoftDirect:
		return []string{string(EventDirectMention), string(EventReplyToBot), string(EventNameMention), string(EventSoftDirect)}
	default:
		return []string{string(EventQuestion), string(EventTechTopic), string(EventHumorTrigger), string(EventSmallTalk)}
	}
}

func (e *Engine) cooldownChannel(eventType EventType) string {
	switch eventType {
	case EventCommand:
		return "command"
	case EventDirectMention, EventReplyToBot, EventNameMention, EventSoftDirect:
		return "direct"
	case EventLocalReaction:
		return "local_reaction"
	case EventIdleProactive:
		return "proactive"
	default:
		return "ambient_llm"
	}
}

func (e *Engine) remainingCooldown(event Event, settings storage.ChatSettings) int {
	cooldown := e.cooldownFor(event.Type, settings)
	last := e.store.LastAnsweredEventAt(event.ChatID, cooldownEventTypes(event.Type)...)
	if cooldown <= 0 || last.IsZero() {
		return 0
	}
	remaining := cooldown - time.Since(last)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Seconds()) + 1
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
