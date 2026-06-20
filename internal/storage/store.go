package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
)

type Store struct {
	mu       sync.Mutex
	path     string
	defaults config.BotConfig
	state    state
}

type state struct {
	Settings map[int64]ChatSettings   `json:"settings"`
	Summaries map[int64]ChatSummary    `json:"summaries"`
	Messages  map[int64][]MessageRecord `json:"messages"`
	Stats     map[int64]Stats          `json:"stats"`
	Events    []EventLog               `json:"events"`
}

func New(path string, defaults config.BotConfig) (*Store, error) {
	store := &Store{
		path:     path,
		defaults: defaults,
		state: state{
			Settings: make(map[int64]ChatSettings),
			Summaries: make(map[int64]ChatSummary),
			Messages:  make(map[int64][]MessageRecord),
			Stats:     make(map[int64]Stats),
		},
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Settings(chatID int64) ChatSettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, ok := s.state.Settings[chatID]
	if !ok {
		settings = s.defaultSettings(chatID)
		s.state.Settings[chatID] = settings
		_ = s.saveLocked()
	}
	return settings
}

func (s *Store) UpdateSettings(chatID int64, mutate func(*ChatSettings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, ok := s.state.Settings[chatID]
	if !ok {
		settings = s.defaultSettings(chatID)
	}
	mutate(&settings)
	settings.UpdatedAt = time.Now().UTC()
	s.state.Settings[chatID] = settings
	return s.saveLocked()
}

func (s *Store) AddMessage(message MessageRecord, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = s.defaults.ContextLimit
	}
	messages := append(s.state.Messages[message.ChatID], message)
	if len(messages) > limit {
		messages = slices.Clone(messages[len(messages)-limit:])
	}
	s.state.Messages[message.ChatID] = messages
	return s.saveLocked()
}

func (s *Store) RecentMessages(chatID int64, limit int) []MessageRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.state.Messages[chatID]
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return slices.Clone(messages)
}

func (s *Store) LastMessageAt(chatID int64) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.state.Messages[chatID]
	if len(messages) == 0 {
		return time.Time{}
	}
	return messages[len(messages)-1].Time
}

func (s *Store) LastBotMessageAt(chatID int64) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.state.Messages[chatID]
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].IsBot {
			return messages[i].Time
		}
	}
	return time.Time{}
}

func (s *Store) CountBotMessagesSince(chatID int64, since time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, message := range s.state.Messages[chatID] {
		if message.IsBot && message.Time.After(since) {
			count++
		}
	}
	return count
}

func (s *Store) CountProactiveSince(chatID int64, since time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, event := range s.state.Events {
		if event.ChatID == chatID && event.EventType == "IDLE_PROACTIVE" && event.Answered && event.Time.After(since) {
			count++
		}
	}
	return count
}

func (s *Store) TokenUsageSince(chatID int64, since time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for _, event := range s.state.Events {
		if event.ChatID == chatID && event.Time.After(since) {
			total += event.InputTokens + event.OutputTokens
		}
	}
	return total
}

func (s *Store) LogEvent(event EventLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Events = append(s.state.Events, event)
	if len(s.state.Events) > 1000 {
		s.state.Events = slices.Clone(s.state.Events[len(s.state.Events)-1000:])
	}

	stats := s.state.Stats[event.ChatID]
	if event.EventType != "IDLE_PROACTIVE" {
		stats.IncomingMessages++
	}
	if event.Answered {
		stats.BotReplies++
		if event.EventType == "IDLE_PROACTIVE" {
			stats.ProactiveMessages++
		}
	} else {
		stats.SkippedEvents++
	}
	stats.InputTokens += event.InputTokens
	stats.OutputTokens += event.OutputTokens
	switch event.ErrorSource {
	case "telegram":
		stats.TelegramAPIErrors++
	case "polza":
		stats.PolzaAPIErrors++
	}
	s.state.Stats[event.ChatID] = stats
	return s.saveLocked()
}

func (s *Store) Stats(chatID int64) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Stats[chatID]
}

func (s *Store) Summary(chatID int64) ChatSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Summaries[chatID]
}

func (s *Store) SetSummary(summary ChatSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Summaries[summary.ChatID] = summary
	return s.saveLocked()
}

func (s *Store) ResetContext(chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Messages, chatID)
	delete(s.state.Summaries, chatID)
	return s.saveLocked()
}

func (s *Store) ChatIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]int64, 0, len(s.state.Settings))
	for id := range s.state.Settings {
		ids = append(ids, id)
	}
	return ids
}

func (s *Store) defaultSettings(chatID int64) ChatSettings {
	return ChatSettings{
		ChatID:             chatID,
		Enabled:            true,
		Mode:               s.defaults.DefaultMode,
		ProactiveEnabled:   true,
		MinDelaySeconds:    int(s.defaults.MinDelay.Seconds()),
		MaxRepliesPerHour:  s.defaults.MaxRepliesPerHour,
		MaxProactivePerDay: s.defaults.MaxProactivePerDay,
		DailyTokenLimit:    s.defaults.DailyTokenLimit,
		StoreText:          s.defaults.PrivacyStoreText,
		UpdatedAt:          time.Now().UTC(),
	}
}

func (s *Store) load() error {
	content, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(content) == 0 {
		return nil
	}
	if err := json.Unmarshal(content, &s.state); err != nil {
		return err
	}
	if s.state.Settings == nil {
		s.state.Settings = make(map[int64]ChatSettings)
	}
	if s.state.Summaries == nil {
		s.state.Summaries = make(map[int64]ChatSummary)
	}
	if s.state.Messages == nil {
		s.state.Messages = make(map[int64][]MessageRecord)
	}
	if s.state.Stats == nil {
		s.state.Stats = make(map[int64]Stats)
	}
	return nil
}

func (s *Store) saveLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".gofer-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
