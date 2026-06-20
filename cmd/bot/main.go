package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	contextx "github.com/w6itec6apel/gofer/internal/context"
	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/decision"
	"github.com/w6itec6apel/gofer/internal/llm"
	"github.com/w6itec6apel/gofer/internal/persona"
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
	commands *telegram.CommandHandler
}

func main() {
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
		commands: telegram.NewCommandHandler(tg, store, cfg.Telegram.AllowedUserID),
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

	decisionResult := a.decider.Decide(message, settings)
	if decisionResult.Event.IsCommand {
		a.handleCommand(ctx, message, decisionResult.Event)
		return
	}
	if !decisionResult.Respond {
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
		Time:      time.Now().UTC(),
		ChatID:    message.Chat.ID,
		EventType: string(event.Type),
		Answered:  err == nil,
		Reason:    "command",
	}
	if message.From != nil {
		log.UserID = message.From.ID
	}
	if err != nil {
		log.Error = err.Error()
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
		{Role: "user", Content: persona.BuildUserPrompt(result.Event, chatContext, settings.Mode)},
	}

	started := time.Now()
	completion, err := a.llm.Complete(ctx, messages)
	eventLog := storage.EventLog{
		Time:      time.Now().UTC(),
		ChatID:    result.Event.ChatID,
		UserID:    result.Event.UserID,
		EventType: string(result.Event.Type),
		Reason:    result.Reason,
		Model:     a.cfg.Polza.Model,
	}
	if err != nil {
		eventLog.Error = err.Error()
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
		a.logger.Error("telegram send failed", "error", err)
		_ = a.store.LogEvent(eventLog)
		return
	}

	a.saveBotMessage(result.Event.ChatID, sent, text)
	_ = a.store.LogEvent(eventLog)
	a.logger.Info("message answered", "chat_id", result.Event.ChatID, "event", result.Event.Type, "latency_ms", time.Since(started).Milliseconds())
}

func (a *app) runProactive(ctx context.Context) {
	for _, chatID := range a.store.ChatIDs() {
		settings := a.store.Settings(chatID)
		result := a.decider.DecideProactive(chatID, settings)
		if !result.Respond {
			_ = a.store.LogEvent(storage.EventLog{
				Time:      time.Now().UTC(),
				ChatID:    chatID,
				EventType: string(result.Event.Type),
				Answered:  false,
				Reason:    result.Reason,
			})
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
