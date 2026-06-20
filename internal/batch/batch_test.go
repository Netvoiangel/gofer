package batch

import (
	"strings"
	"testing"
	"time"
)

func TestBufferCollectsMessageBatch(t *testing.T) {
	manager := New(20*time.Second, 5)
	manager.Add(Message{ChatID: 1, Text: "А я ansible написал", EventType: "TECH_TOPIC", Time: time.Now()})
	manager.Add(Message{ChatID: 1, Text: "Предлагаю создать интерфейс с VPN", EventType: "TECH_TOPIC", Time: time.Now()})
	manager.Add(Message{ChatID: 1, Text: "У меня сломался сервер", EventType: "TECH_TOPIC", Time: time.Now()})

	if got := manager.Len(1); got != 3 {
		t.Fatalf("expected 3 pending messages, got %d", got)
	}
}

func TestFlushReturnsOneCombinedBatch(t *testing.T) {
	manager := New(20*time.Second, 5)
	manager.Add(Message{ChatID: 1, Text: "ansible", EventType: "TECH_TOPIC", Time: time.Now()})
	manager.Add(Message{ChatID: 1, Text: "vpn", EventType: "TECH_TOPIC", Time: time.Now()})
	manager.Add(Message{ChatID: 1, Text: "server broken", EventType: "TECH_TOPIC", Time: time.Now()})

	batch, ok := manager.Flush(1)
	if !ok {
		t.Fatalf("expected batch")
	}
	if len(batch.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(batch.Messages))
	}
	if !strings.Contains(batch.Text, "ansible") || !strings.Contains(batch.Text, "vpn") || !strings.Contains(batch.Text, "server broken") {
		t.Fatalf("unexpected combined text: %s", batch.Text)
	}
}
