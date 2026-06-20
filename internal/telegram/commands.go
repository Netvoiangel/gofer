package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/storage"
)

type CommandHandler struct {
	client        *Client
	store         *storage.Store
	botConfig     config.BotConfig
	allowedUserID map[int64]struct{}
}

func NewCommandHandler(client *Client, store *storage.Store, botConfig config.BotConfig, allowedUserID map[int64]struct{}) *CommandHandler {
	return &CommandHandler{client: client, store: store, botConfig: botConfig, allowedUserID: allowedUserID}
}

func (h *CommandHandler) Handle(ctx context.Context, message Message) (string, bool) {
	if strings.TrimSpace(message.Text) == "" || !strings.HasPrefix(message.Text, "/") {
		return "", false
	}

	command, args := splitCommand(message.Text)
	switch command {
	case "/gopher_help", "/start":
		return helpText(), true
	case "/gopher_stats":
		stats := h.store.Stats(message.Chat.ID)
		return fmt.Sprintf("Статистика Гофера: входящих %d, ответов %d, инициативных %d, пропущено %d, токены %d/%d.",
			stats.IncomingMessages, stats.BotReplies, stats.ProactiveMessages, stats.SkippedEvents, stats.InputTokens, stats.OutputTokens), true
	case "/gopher_budget":
		settings := h.store.Settings(message.Chat.ID)
		return fmt.Sprintf("Режим: %s\nРазговорчивость: %s\nМат: %s\n\nCooldown:\n- command: %d сек\n- direct/reply: %d сек\n- ambient LLM: %d сек\n- local reaction: %d сек\n- proactive: %d сек\n- debounce: %d сек\n\nЛимиты:\n- максимум ответов в час: %d\n- инициативных в день: %d\n- дневной бюджет токенов: %d",
			settings.Mode,
			h.botConfig.Chattiness,
			h.botConfig.ProfanityLevel,
			int(h.botConfig.CommandCooldown.Seconds()),
			int(h.botConfig.DirectCooldown.Seconds()),
			int(h.botConfig.AmbientCooldown.Seconds()),
			int(h.botConfig.LocalCooldown.Seconds()),
			int(h.botConfig.ProactiveCooldown.Seconds()),
			int(h.botConfig.Debounce.Seconds()),
			settings.MaxRepliesPerHour,
			settings.MaxProactivePerDay,
			settings.DailyTokenLimit), true
	}

	admin, err := h.isAdmin(ctx, message)
	if err != nil {
		return "Не смог проверить права администратора. Попробуйте позже.", true
	}
	if !admin {
		return "Эту команду может выполнить только администратор чата.", true
	}

	switch command {
	case "/gopher_on":
		_ = h.store.UpdateSettings(message.Chat.ID, func(settings *storage.ChatSettings) {
			settings.Enabled = true
		})
		return "Гофер снова на связи.", true
	case "/gopher_off":
		_ = h.store.UpdateSettings(message.Chat.ID, func(settings *storage.ChatSettings) {
			settings.Enabled = false
		})
		return "Гофер ушёл в тихий режим.", true
	case "/gopher_silent":
		minutes := 60
		if len(args) > 0 {
			if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
				minutes = parsed
			}
		}
		_ = h.store.UpdateSettings(message.Chat.ID, func(settings *storage.ChatSettings) {
			settings.SilentUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
		})
		return fmt.Sprintf("Инициативные реакции выключены на %d мин.", minutes), true
	case "/gopher_mode":
		if len(args) == 0 {
			return "Укажите режим: calm, funny, tech или angry.", true
		}
		mode := strings.ToLower(args[0])
		if mode != "calm" && mode != "funny" && mode != "tech" && mode != "angry" {
			return "Не знаю такой режим. Доступны: calm, funny, tech, angry.", true
		}
		_ = h.store.UpdateSettings(message.Chat.ID, func(settings *storage.ChatSettings) {
			settings.Mode = mode
		})
		return "Режим Гофера: " + mode + ".", true
	case "/gopher_reset_context":
		_ = h.store.ResetContext(message.Chat.ID)
		return "Краткосрочный контекст очищен.", true
	default:
		return "Неизвестная команда. Напишите /gopher_help.", true
	}
}

func (h *CommandHandler) isAdmin(ctx context.Context, message Message) (bool, error) {
	if message.From == nil {
		return false, nil
	}
	if _, ok := h.allowedUserID[message.From.ID]; ok {
		return true, nil
	}
	return h.client.IsChatAdmin(ctx, message.Chat.ID, message.From.ID)
}

func splitCommand(text string) (string, []string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", nil
	}
	command := strings.Split(fields[0], "@")[0]
	return strings.ToLower(command), fields[1:]
}

func helpText() string {
	return strings.Join([]string{
		"Команды Гофера:",
		"/gopher_help — список команд",
		"/gopher_on — включить активность",
		"/gopher_off — выключить активность",
		"/gopher_silent 60 — замолчать на 60 минут",
		"/gopher_mode calm|funny|tech|angry — сменить стиль",
		"/gopher_stats — статистика",
		"/gopher_budget — текущие лимиты",
		"/gopher_reset_context — очистить краткосрочный контекст",
	}, "\n")
}
