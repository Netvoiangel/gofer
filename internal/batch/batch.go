package batch

import (
	"strings"
	"sync"
	"time"
)

type Message struct {
	ChatID    int64
	UserID    int64
	MessageID int
	EventType string
	Text      string
	Time      time.Time
}

type Batch struct {
	ChatID    int64
	UserID    int64
	MessageID int
	EventType string
	Text      string
	Messages  []Message
}

type Manager struct {
	mu          sync.Mutex
	window      time.Duration
	maxMessages int
	pending     map[int64][]Message
}

func New(window time.Duration, maxMessages int) *Manager {
	if maxMessages <= 0 {
		maxMessages = 5
	}
	return &Manager{
		window:      window,
		maxMessages: maxMessages,
		pending:     make(map[int64][]Message),
	}
}

func (m *Manager) Add(message Message) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := message.Time
	if now.IsZero() {
		now = time.Now().UTC()
		message.Time = now
	}
	messages := m.pending[message.ChatID]
	if m.window > 0 {
		cutoff := now.Add(-m.window)
		filtered := messages[:0]
		for _, existing := range messages {
			if existing.Time.After(cutoff) {
				filtered = append(filtered, existing)
			}
		}
		messages = filtered
	}
	messages = append(messages, message)
	if len(messages) > m.maxMessages {
		messages = messages[len(messages)-m.maxMessages:]
	}
	m.pending[message.ChatID] = messages
	return len(messages)
}

func (m *Manager) Flush(chatID int64) (Batch, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	messages := m.pending[chatID]
	if len(messages) == 0 {
		return Batch{}, false
	}
	delete(m.pending, chatID)

	var builder strings.Builder
	for i, message := range messages {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(message.Text)
	}
	last := messages[len(messages)-1]
	return Batch{
		ChatID:    chatID,
		UserID:    last.UserID,
		MessageID: last.MessageID,
		EventType: highestPriorityEvent(messages),
		Text:      builder.String(),
		Messages:  append([]Message(nil), messages...),
	}, true
}

func (m *Manager) Len(chatID int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending[chatID])
}

func highestPriorityEvent(messages []Message) string {
	priority := map[string]int{
		"SOFT_DIRECT":   100,
		"QUESTION":      80,
		"TECH_TOPIC":    70,
		"HUMOR_TRIGGER": 60,
		"SMALL_TALK":    10,
	}
	best := "SMALL_TALK"
	bestScore := 0
	for _, message := range messages {
		if score := priority[message.EventType]; score > bestScore {
			best = message.EventType
			bestScore = score
		}
	}
	return best
}
