package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Telegram TelegramConfig
	Polza    PolzaConfig
	Bot      BotConfig
	Storage  StorageConfig
	LogLevel slog.Level
}

type TelegramConfig struct {
	Token         string
	PollTimeout   int
	AllowedUserID map[int64]struct{}
}

type PolzaConfig struct {
	APIKey          string
	BaseURL         string
	Model           string
	Temperature     float64
	MaxTokens       int
	Timeout         time.Duration
	RetryCount      int
	SilentOnMissing bool
}

type BotConfig struct {
	Username            string
	NameTriggers        []string
	DefaultMode         string
	DevMode             bool
	Chattiness          string
	ProfanityLevel      string
	MinDelay            time.Duration
	CommandCooldown     time.Duration
	DirectCooldown      time.Duration
	AmbientCooldown     time.Duration
	LocalCooldown       time.Duration
	ProactiveCooldown   time.Duration
	SoftDirectWindow    time.Duration
	Debounce            time.Duration
	BatchWindow         time.Duration
	BatchMaxMessages    int
	MaxRepliesPerHour   int
	MaxProactivePerDay  int
	DailyTokenLimit     int
	ContextLimit        int
	MaxContextTokens    int
	ProactiveInterval   time.Duration
	ProactiveIdleAfter  time.Duration
	PrivacyStoreText    bool
	ResponseProbability ProbabilityConfig
}

type ProbabilityConfig struct {
	Question     float64
	GoTopic      float64
	TechTopic    float64
	HumorTrigger float64
	SmallTalk    float64
	ProactiveMin float64
	ProactiveMax float64
}

type StorageConfig struct {
	Path string
}

func Load() (Config, error) {
	devMode := envBool("BOT_DEV_MODE", false)
	cfg := Config{
		Telegram: TelegramConfig{
			Token:         strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			PollTimeout:   envInt("TELEGRAM_POLL_TIMEOUT", 25),
			AllowedUserID: parseInt64Set(os.Getenv("ADMIN_USER_IDS")),
		},
		Polza: PolzaConfig{
			APIKey:          strings.TrimSpace(os.Getenv("POLZA_API_KEY")),
			BaseURL:         envString("POLZA_BASE_URL", "https://api.polza.ai/api/v1"),
			Model:           envString("POLZA_MODEL", "openai/gpt-4o-mini"),
			Temperature:     envFloat("POLZA_TEMPERATURE", 0.7),
			MaxTokens:       envInt("POLZA_MAX_TOKENS", 400),
			Timeout:         time.Duration(envInt("POLZA_TIMEOUT_SECONDS", 20)) * time.Second,
			RetryCount:      envInt("POLZA_RETRY_COUNT", 2),
			SilentOnMissing: envBool("POLZA_SILENT_ON_MISSING", true),
		},
		Bot: BotConfig{
			Username:           strings.TrimPrefix(envString("BOT_USERNAME", ""), "@"),
			NameTriggers:       envList("BOT_NAME_TRIGGERS", []string{"гофер", "gopher", "бот"}),
			DefaultMode:        envString("BOT_DEFAULT_MODE", "funny"),
			DevMode:            devMode,
			Chattiness:         envString("BOT_CHATTINESS", "medium"),
			ProfanityLevel:     envString("BOT_PROFANITY_LEVEL", "low"),
			MinDelay:           time.Duration(envInt("BOT_MIN_DELAY_SECONDS", 180)) * time.Second,
			CommandCooldown:    time.Duration(envInt("BOT_COMMAND_COOLDOWN_SECONDS", 3)) * time.Second,
			DirectCooldown:     time.Duration(envInt("BOT_DIRECT_COOLDOWN_SECONDS", 60)) * time.Second,
			AmbientCooldown:    time.Duration(envInt("BOT_AMBIENT_LLM_COOLDOWN_SECONDS", envInt("BOT_MIN_DELAY_SECONDS", 180))) * time.Second,
			LocalCooldown:      time.Duration(envInt("BOT_LOCAL_REACTION_COOLDOWN_SECONDS", 120)) * time.Second,
			ProactiveCooldown:  time.Duration(envInt("BOT_PROACTIVE_COOLDOWN_SECONDS", 14400)) * time.Second,
			SoftDirectWindow:   time.Duration(envInt("BOT_SOFT_DIRECT_WINDOW_SECONDS", 300)) * time.Second,
			Debounce:           time.Duration(envInt("BOT_DEBOUNCE_SECONDS", 8)) * time.Second,
			BatchWindow:        time.Duration(envInt("BOT_BATCH_WINDOW_SECONDS", 20)) * time.Second,
			BatchMaxMessages:   envInt("BOT_BATCH_MAX_MESSAGES", 5),
			MaxRepliesPerHour:  envInt("BOT_MAX_REPLIES_PER_HOUR", 12),
			MaxProactivePerDay: envInt("BOT_MAX_PROACTIVE_PER_DAY", 2),
			DailyTokenLimit:    envInt("BOT_DAILY_TOKEN_LIMIT", 20000),
			ContextLimit:       envInt("BOT_CONTEXT_LIMIT", 50),
			MaxContextTokens:   envInt("BOT_MAX_CONTEXT_TOKENS", 1200),
			ProactiveInterval:  time.Duration(envInt("BOT_PROACTIVE_INTERVAL_SECONDS", 1800)) * time.Second,
			ProactiveIdleAfter: time.Duration(envInt("BOT_PROACTIVE_IDLE_AFTER_SECONDS", 10800)) * time.Second,
			PrivacyStoreText:   envBool("BOT_STORE_TEXT", true),
			ResponseProbability: ProbabilityConfig{
				Question:     envFloat("PROB_QUESTION", 0.55),
				GoTopic:      envFloat("PROB_GO_TOPIC", 0.60),
				TechTopic:    envFloat("PROB_TECH_TOPIC", 0.35),
				HumorTrigger: envFloat("PROB_HUMOR_TRIGGER", 0.20),
				SmallTalk:    envFloat("PROB_SMALL_TALK", 0.04),
				ProactiveMin: envFloat("PROB_PROACTIVE_MIN", 0.02),
				ProactiveMax: envFloat("PROB_PROACTIVE_MAX", 0.06),
			},
		},
		Storage:  StorageConfig{Path: envString("STORAGE_PATH", envString("DATABASE_URL", "data/state.json"))},
		LogLevel: parseLogLevel(envString("LOG_LEVEL", "info")),
	}

	if cfg.Bot.DevMode {
		cfg.Bot.Chattiness = "high"
		cfg.Bot.CommandCooldown = time.Second
		cfg.Bot.DirectCooldown = 10 * time.Second
		cfg.Bot.AmbientCooldown = 20 * time.Second
		cfg.Bot.LocalCooldown = 10 * time.Second
		cfg.Bot.ProactiveCooldown = 30 * time.Minute
		cfg.Bot.Debounce = 4 * time.Second
		cfg.Bot.MaxRepliesPerHour = 60
		cfg.Bot.ResponseProbability.Question = 1
		cfg.Bot.ResponseProbability.GoTopic = 1
		cfg.Bot.ResponseProbability.TechTopic = 0.80
		cfg.Bot.ResponseProbability.HumorTrigger = 0.60
		cfg.Bot.ResponseProbability.SmallTalk = 0.20
	}

	if cfg.Telegram.Token == "" {
		return cfg, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.Polza.APIKey == "" && !cfg.Polza.SilentOnMissing {
		return cfg, fmt.Errorf("POLZA_API_KEY is required")
	}
	return cfg, nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envList(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func parseInt64Set(value string) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		parsed, err := strconv.ParseInt(item, 10, 64)
		if err == nil {
			result[parsed] = struct{}{}
		}
	}
	return result
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
