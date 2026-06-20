package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/w6itec6apel/gofer/internal/batch"
	"github.com/w6itec6apel/gofer/internal/config"
	contextx "github.com/w6itec6apel/gofer/internal/context"
	"github.com/w6itec6apel/gofer/internal/decision"
	"github.com/w6itec6apel/gofer/internal/llm"
	"github.com/w6itec6apel/gofer/internal/persona"
	"github.com/w6itec6apel/gofer/internal/reactions"
	"github.com/w6itec6apel/gofer/internal/scheduler"
	"github.com/w6itec6apel/gofer/internal/storage"
	"github.com/w6itec6apel/gofer/internal/telegram"
)

type app struct {
	cfg      config.Config
	logger   *slog.Logger
	tg       *telegram.Client
	llm      *llm.Client
	store    *storage.Store
	ctx      *contextx.Manager
	decider  *decision.Engine
	batches  *batch.Manager
	timers   map[int64]*time.Timer
	timerMu  sync.Mutex
	commands *telegram.CommandHandler
}

func main() {
	config.LoadDotEnv(".env")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	store, err := storage.New(cfg.Storage.Path, cfg.Bot)
	if err != nil {
		logger.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	tg := telegram.NewClient(cfg.Telegram.Token)
	if cfg.Bot.Username == "" {
		if me, err := tg.GetMe(context.Background()); err == nil && me != nil {
			cfg.Bot.Username = me.Username
			logger.Info("bot username detected", "username", cfg.Bot.Username)
		} else {
			logger.Warn("bot username was not detected", "error", err)
		}
	}

	application := &app{
		cfg:      cfg,
		logger:   logger,
		tg:       tg,
		llm:      llm.NewClient(cfg.Polza),
		store:    store,
		ctx:      contextx.NewManager(store, cfg.Bot.ContextLimit, cfg.Bot.MaxContextTokens),
		decider:  decision.NewEngine(cfg.Bot, store),
		batches:  batch.New(cfg.Bot.BatchWindow, cfg.Bot.BatchMaxMessages),
		timers:   make(map[int64]*time.Timer),
		commands: telegram.NewCommandHandler(tg, store, cfg.Bot, cfg.Telegram.AllowedUserID),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go scheduler.New(cfg.Bot.ProactiveInterval, logger).Run(ctx, application.runProactive)

	if err := application.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}

func (a *app) run(ctx context.Context) error {
	offset := 0
	a.logger.Info("gofer started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := a.tg.GetUpdates(ctx, offset, a.cfg.Telegram.PollTimeout)
		if err != nil {
			a.logger.Error("telegram polling failed", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message != nil {
				a.handleMessage(ctx, *update.Message)
			}
		}
	}
}

func (a *app) handleMessage(ctx context.Context, message telegram.Message) {
	if strings.TrimSpace(message.Text) == "" || message.From == nil {
		return
	}
	if message.From.IsBot {
		return
	}

	settings := a.store.Settings(message.Chat.ID)
	record := storage.MessageRecord{
		Time:      time.Unix(message.Date, 0).UTC(),
		ChatID:    message.Chat.ID,
		UserID:    message.From.ID,
		UserName:  displayName(*message.From),
		Text:      message.Text,
		IsBot:     false,
		MessageID: message.MessageID,
	}
	if err := a.ctx.Add(record, settings.StoreText); err != nil {
		a.logger.Error("message context save failed", "error", err)
	}
	_ = a.ctx.MaybeUpdateSummary(message.Chat.ID)

	event := a.decider.Classify(message)
	if event.IsCommand {
		decisionResult := a.decider.DecideEvent(event, settings)
		if !decisionResult.Respond {
			a.logSkipped(message.Chat.ID, decisionResult, settings)
			_ = a.store.LogEvent(storage.EventLog{
				Time:         time.Now().UTC(),
				ChatID:       message.Chat.ID,
				UserID:       record.UserID,
				EventType:    string(decisionResult.Event.Type),
				Answered:     false,
				Reason:       decisionResult.Reason,
				ResponseKind: "command",
			})
			return
		}
		a.handleCommand(ctx, message, event)
		return
	}
	if a.tryLocalReaction(ctx, message, event, settings) {
		return
	}
	if isAmbientBatchEvent(event.Type) {
		a.queueAmbient(ctx, event, message)
		return
	}

	decisionResult := a.decider.DecideEvent(event, settings)
	if !decisionResult.Respond {
		a.logSkipped(message.Chat.ID, decisionResult, settings)
		_ = a.store.LogEvent(storage.EventLog{
			Time:      time.Now().UTC(),
			ChatID:    message.Chat.ID,
			UserID:    record.UserID,
			EventType: string(decisionResult.Event.Type),
			Answered:  false,
			Reason:    decisionResult.Reason,
		})
		return
	}

	a.respondWithLLM(ctx, decisionResult, settings, message.MessageID)
}

func (a *app) queueAmbient(ctx context.Context, event decision.Event, message telegram.Message) {
	count := a.batches.Add(batch.Message{
		ChatID:    event.ChatID,
		UserID:    event.UserID,
		MessageID: event.MessageID,
		EventType: string(event.Type),
		Text:      event.Text,
		Time:      time.Now().UTC(),
	})
	a.logger.Info("message queued", "chat_id", event.ChatID, "event", event.Type, "reason", "debounce", "queue", "ambient", "pending", count)

	a.timerMu.Lock()
	if timer := a.timers[event.ChatID]; timer != nil {
		timer.Stop()
	}
	a.timers[event.ChatID] = time.AfterFunc(a.cfg.Bot.Debounce, func() {
		a.processAmbientBatch(ctx, event.ChatID)
	})
	a.timerMu.Unlock()
}

func (a *app) processAmbientBatch(ctx context.Context, chatID int64) {
	a.timerMu.Lock()
	delete(a.timers, chatID)
	a.timerMu.Unlock()

	queued, ok := a.batches.Flush(chatID)
	if !ok {
		return
	}
	settings := a.store.Settings(chatID)
	event := decision.Event{
		Type:      decision.EventType(queued.EventType),
		Text:      queued.Text,
		ChatID:    chatID,
		UserID:    queued.UserID,
		MessageID: queued.MessageID,
	}
	result := a.decider.DecideEvent(event, settings)
	if !result.Respond {
		if result.Reason == "cooldown" && isQueueableEvent(event.Type) {
			a.logger.Info("message queued", "chat_id", chatID, "event", event.Type, "reason", "queued_due_to_cooldown", "queue", "ambient", "remaining_seconds", result.RemainingSeconds)
			_ = a.store.LogEvent(storage.EventLog{
				Time:         time.Now().UTC(),
				ChatID:       chatID,
				UserID:       event.UserID,
				EventType:    string(event.Type),
				Answered:     false,
				Reason:       "queued_due_to_cooldown",
				ResponseKind: "queued",
			})
			delay := time.Duration(result.RemainingSeconds) * time.Second
			if delay <= 0 {
				delay = a.cfg.Bot.Debounce
			}
			a.batches.Add(batch.Message{
				ChatID:    chatID,
				UserID:    event.UserID,
				MessageID: event.MessageID,
				EventType: string(event.Type),
				Text:      event.Text,
				Time:      time.Now().UTC(),
			})
			time.AfterFunc(delay, func() {
				a.processAmbientBatch(ctx, chatID)
			})
			return
		}
		a.logSkipped(chatID, result, settings)
		_ = a.store.LogEvent(storage.EventLog{
			Time:         time.Now().UTC(),
			ChatID:       chatID,
			UserID:       event.UserID,
			EventType:    string(event.Type),
			Answered:     false,
			Reason:       result.Reason,
			ResponseKind: "ambient_batch",
		})
		return
	}
	a.respondWithLLM(ctx, result, settings, queued.MessageID)
}

func (a *app) logSkipped(chatID int64, result decision.Decision, settings storage.ChatSettings) {
	attrs := []any{
		"chat_id", chatID,
		"event", result.Event.Type,
		"reason", result.Reason,
		"probability", result.Probability,
		"roll", result.Roll,
		"mode", settings.Mode,
		"chattiness", a.cfg.Bot.Chattiness,
		"cooldown_channel", result.CooldownChannel,
	}
	if result.RemainingSeconds > 0 {
		attrs = append(attrs, "remaining_seconds", result.RemainingSeconds)
	}
	a.logger.Info("message skipped", attrs...)
}

func (a *app) handleCommand(ctx context.Context, message telegram.Message, event decision.Event) {
	response, handled := a.commands.Handle(ctx, message)
	if !handled {
		return
	}
	if strings.TrimSpace(response) == "" {
		return
	}
	sent, err := a.tg.SendMessage(ctx, message.Chat.ID, response, message.MessageID)
	log := storage.EventLog{
		Time:         time.Now().UTC(),
		ChatID:       message.Chat.ID,
		EventType:    string(event.Type),
		Answered:     err == nil,
		Reason:       "command",
		ResponseKind: "command",
	}
	if message.From != nil {
		log.UserID = message.From.ID
	}
	if err != nil {
		log.Error = err.Error()
		log.ErrorSource = "telegram"
		a.logger.Error("command response failed", "error", err)
	} else {
		a.saveBotMessage(message.Chat.ID, sent, response)
	}
	_ = a.store.LogEvent(log)
}

func (a *app) respondWithLLM(ctx context.Context, result decision.Decision, settings storage.ChatSettings, replyTo int) {
	chatContext := a.ctx.Build(result.Event.ChatID)
	messages := []llm.Message{
		{Role: "system", Content: persona.SystemPrompt},
		{Role: "user", Content: persona.BuildUserPrompt(result.Event, chatContext, settings.Mode, a.cfg.Bot.ProfanityLevel)},
	}

	started := time.Now()
	completion, err := a.llm.Complete(ctx, messages)
	eventLog := storage.EventLog{
		Time:         time.Now().UTC(),
		ChatID:       result.Event.ChatID,
		UserID:       result.Event.UserID,
		EventType:    string(result.Event.Type),
		Reason:       result.Reason,
		Model:        a.cfg.Polza.Model,
		ResponseKind: "llm",
	}
	if err != nil {
		eventLog.Error = err.Error()
		eventLog.ErrorSource = "polza"
		a.logger.Error("llm completion failed", "error", err)
		_ = a.store.LogEvent(eventLog)
		return
	}

	text := persona.PostProcess(completion.Text)
	if text == "" {
		eventLog.Reason = "llm_silence"
		_ = a.store.LogEvent(eventLog)
		return
	}

	sent, err := a.tg.SendMessage(ctx, result.Event.ChatID, text, replyTo)
	eventLog.Model = completion.Model
	eventLog.InputTokens = completion.InputTokens
	eventLog.OutputTokens = completion.OutputTokens
	eventLog.Answered = err == nil
	if err != nil {
		eventLog.Error = err.Error()
		eventLog.ErrorSource = "telegram"
		a.logger.Error("telegram send failed", "error", err)
		_ = a.store.LogEvent(eventLog)
		return
	}

	a.saveBotMessage(result.Event.ChatID, sent, text)
	_ = a.store.LogEvent(eventLog)
	a.logger.Info("message answered", "chat_id", result.Event.ChatID, "event", result.Event.Type, "latency_ms", time.Since(started).Milliseconds())
}

func (a *app) tryLocalReaction(ctx context.Context, message telegram.Message, event decision.Event, settings storage.ChatSettings) bool {
	if event.Type == decision.EventDirectMention || event.Type == decision.EventReplyToBot || event.Type == decision.EventNameMention || event.Type == decision.EventQuestion {
		return false
	}

	candidate, ok := reactions.Match(event.Text, a.cfg.Bot.Chattiness, a.cfg.Bot.ProfanityLevel)
	if !ok {
		return false
	}

	allowed, reason := a.decider.CanLocalReact(message.Chat.ID, settings)
	if !allowed {
		a.logger.Info("local reaction skipped", "chat_id", message.Chat.ID, "event", decision.EventLocalReaction, "reason", reason, "topic", candidate.Topic, "trigger", candidate.Trigger, "llm_used", false)
		_ = a.store.LogEvent(storage.EventLog{
			Time:         time.Now().UTC(),
			ChatID:       message.Chat.ID,
			UserID:       event.UserID,
			EventType:    string(decision.EventLocalReaction),
			Answered:     false,
			Reason:       reason,
			ResponseKind: "local",
		})
		return true
	}

	sent, err := a.tg.SendMessage(ctx, message.Chat.ID, candidate.Text, message.MessageID)
	eventLog := storage.EventLog{
		Time:         time.Now().UTC(),
		ChatID:       message.Chat.ID,
		UserID:       event.UserID,
		EventType:    string(decision.EventLocalReaction),
		Answered:     err == nil,
		Reason:       "local_" + candidate.Topic,
		ResponseKind: "local",
	}
	if err != nil {
		eventLog.Error = err.Error()
		eventLog.ErrorSource = "telegram"
		a.logger.Error("local reaction failed", "error", err)
		_ = a.store.LogEvent(eventLog)
		return true
	}

	a.saveBotMessage(message.Chat.ID, sent, candidate.Text)
	_ = a.store.LogEvent(eventLog)
	a.logger.Info("local reaction sent", "chat_id", message.Chat.ID, "event", decision.EventLocalReaction, "topic", candidate.Topic, "trigger", candidate.Trigger, "chance", candidate.Chance, "llm_used", false)
	return true
}

func (a *app) runProactive(ctx context.Context) {
	for _, chatID := range a.store.ChatIDs() {
		settings := a.store.Settings(chatID)
		result := a.decider.DecideProactive(chatID, settings)
		if !result.Respond {
			a.logger.Debug("proactive skipped", "chat_id", chatID, "reason", result.Reason)
			continue
		}
		a.respondWithLLM(ctx, result, settings, 0)
	}
}

func (a *app) saveBotMessage(chatID int64, message *telegram.Message, text string) {
	record := storage.MessageRecord{
		Time:   time.Now().UTC(),
		ChatID: chatID,
		Text:   text,
		IsBot:  true,
	}
	if message != nil {
		record.MessageID = message.MessageID
		if message.From != nil {
			record.UserID = message.From.ID
			record.UserName = displayName(*message.From)
		}
	}
	settings := a.store.Settings(chatID)
	if err := a.ctx.Add(record, settings.StoreText); err != nil {
		a.logger.Error("bot context save failed", "error", err)
	}
}

func displayName(user telegram.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name != "" {
		return name
	}
	return "user"
}

func isAmbientBatchEvent(eventType decision.EventType) bool {
	switch eventType {
	case decision.EventQuestion, decision.EventTechTopic, decision.EventHumorTrigger, decision.EventSmallTalk:
		return true
	default:
		return false
	}
}

func isQueueableEvent(eventType decision.EventType) bool {
	switch eventType {
	case decision.EventQuestion, decision.EventTechTopic, decision.EventHumorTrigger, decision.EventSoftDirect:
		return true
	default:
		return false
	}
}
