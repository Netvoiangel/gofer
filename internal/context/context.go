package contextx

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/w6itec6apel/gofer/internal/storage"
)

type Manager struct {
	store            *storage.Store
	contextLimit     int
	maxContextTokens int
}

func NewManager(store *storage.Store, contextLimit int, maxContextTokens int) *Manager {
	return &Manager{store: store, contextLimit: contextLimit, maxContextTokens: maxContextTokens}
}

func (m *Manager) Add(message storage.MessageRecord, storeText bool) error {
	if !storeText {
		message.Text = ""
	}
	return m.store.AddMessage(message, m.contextLimit)
}

func (m *Manager) Build(chatID int64) string {
	messages := m.store.RecentMessages(chatID, m.contextLimit)
	summary := m.store.Summary(chatID)

	var builder strings.Builder
	if strings.TrimSpace(summary.Summary) != "" {
		builder.WriteString("Краткое резюме чата: ")
		builder.WriteString(summary.Summary)
		builder.WriteString("\n")
	}

	builder.WriteString("Последние сообщения:\n")
	tokenBudget := m.maxContextTokens
	selected := make([]string, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		line := formatMessage(message)
		roughTokens := len([]rune(line)) / 4
		if roughTokens < 1 {
			roughTokens = 1
		}
		if tokenBudget-roughTokens < 0 {
			break
		}
		tokenBudget -= roughTokens
		selected = append(selected, line)
	}
	slices.Reverse(selected)
	for _, line := range selected {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func (m *Manager) MaybeUpdateSummary(chatID int64) error {
	messages := m.store.RecentMessages(chatID, 12)
	topics := detectTopics(messages)
	if len(topics) == 0 {
		return nil
	}

	summary := "Недавно обсуждали: " + strings.Join(topics, ", ") + "."
	return m.store.SetSummary(storage.ChatSummary{
		ChatID:        chatID,
		Summary:       summary,
		Topics:        topics,
		LastUpdatedAt: time.Now().UTC(),
	})
}

func formatMessage(message storage.MessageRecord) string {
	author := message.UserName
	if author == "" {
		author = fmt.Sprintf("user_%d", message.UserID)
	}
	if message.IsBot {
		author = "Гофер"
	}
	return fmt.Sprintf("[%s] %s: %s", message.Time.Format("15:04"), author, strings.TrimSpace(message.Text))
}

func detectTopics(messages []storage.MessageRecord) []string {
	seen := make(map[string]struct{})
	var topics []string
	keywords := map[string][]string{
		"Go":       {"go", "golang", "goroutine", "channel", "context", "interface"},
		"деплой":   {"deploy", "деплой", "docker", "kubernetes", "k8s", "prod"},
		"ошибки":   {"bug", "баг", "ошибка", "panic", "exception", "trace"},
		"серверы":  {"server", "сервер", "backend", "api", "http", "grpc"},
		"ревью":    {"review", "ревью", "pull request", "merge request"},
	}
	for _, message := range messages {
		lower := strings.ToLower(message.Text)
		for topic, words := range keywords {
			if _, ok := seen[topic]; ok {
				continue
			}
			for _, word := range words {
				if strings.Contains(lower, word) {
					seen[topic] = struct{}{}
					topics = append(topics, topic)
					break
				}
			}
		}
	}
	return topics
}
