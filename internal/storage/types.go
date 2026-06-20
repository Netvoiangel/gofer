package storage

import "time"

type ChatSettings struct {
	ChatID             int64     `json:"chat_id"`
	Enabled            bool      `json:"enabled"`
	Mode               string    `json:"mode"`
	ProactiveEnabled   bool      `json:"proactive_enabled"`
	SilentUntil        time.Time `json:"silent_until,omitempty"`
	MinDelaySeconds    int       `json:"min_delay_seconds"`
	MaxRepliesPerHour  int       `json:"max_replies_per_hour"`
	MaxProactivePerDay int       `json:"max_proactive_per_day"`
	DailyTokenLimit    int       `json:"daily_token_limit"`
	StoreText          bool      `json:"store_text"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ChatSummary struct {
	ChatID        int64     `json:"chat_id"`
	Summary       string    `json:"summary"`
	Topics        []string  `json:"topics"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

type EventLog struct {
	Time         time.Time `json:"time"`
	ChatID       int64     `json:"chat_id"`
	UserID       int64     `json:"user_id,omitempty"`
	EventType    string    `json:"event_type"`
	Answered     bool      `json:"answered"`
	Reason       string    `json:"reason,omitempty"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Error        string    `json:"error,omitempty"`
	ErrorSource  string    `json:"error_source,omitempty"`
}

type Stats struct {
	IncomingMessages   int `json:"incoming_messages"`
	BotReplies         int `json:"bot_replies"`
	ProactiveMessages  int `json:"proactive_messages"`
	SkippedEvents      int `json:"skipped_events"`
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TelegramAPIErrors  int `json:"telegram_api_errors"`
	PolzaAPIErrors     int `json:"polza_api_errors"`
	ResponseTimeMillis int `json:"response_time_millis"`
}

type MessageRecord struct {
	Time      time.Time `json:"time"`
	ChatID    int64     `json:"chat_id"`
	UserID    int64     `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	Text      string    `json:"text,omitempty"`
	IsBot     bool      `json:"is_bot"`
	MessageID int       `json:"message_id"`
}
