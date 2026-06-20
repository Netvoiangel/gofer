package contextx

import (
	"strings"
	"testing"
	"time"

	"github.com/w6itec6apel/gofer/internal/config"
	"github.com/w6itec6apel/gofer/internal/storage"
)

func TestBuildUsesOnlyCurrentChat(t *testing.T) {
	store := newContextTestStore(t)
	manager := NewManager(store, 10, 200)

	_ = manager.Add(storage.MessageRecord{
		Time:   time.Now().UTC(),
		ChatID: 1,
		UserID: 10,
		Text:   "секрет первого чата",
	}, true)
	_ = manager.Add(storage.MessageRecord{
		Time:   time.Now().UTC(),
		ChatID: 2,
		UserID: 20,
		Text:   "сообщение второго чата",
	}, true)

	context := manager.Build(2)
	if strings.Contains(context, "секрет первого чата") {
		t.Fatalf("context leaked message from another chat: %s", context)
	}
	if !strings.Contains(context, "сообщение второго чата") {
		t.Fatalf("context missed current chat message: %s", context)
	}
}

func TestContextLimitKeepsRecentMessages(t *testing.T) {
	store := newContextTestStore(t)
	manager := NewManager(store, 2, 200)

	_ = manager.Add(storage.MessageRecord{Time: time.Now().UTC(), ChatID: 1, UserID: 10, Text: "old"}, true)
	_ = manager.Add(storage.MessageRecord{Time: time.Now().UTC(), ChatID: 1, UserID: 10, Text: "middle"}, true)
	_ = manager.Add(storage.MessageRecord{Time: time.Now().UTC(), ChatID: 1, UserID: 10, Text: "new"}, true)

	context := manager.Build(1)
	if strings.Contains(context, "old") {
		t.Fatalf("context kept an old message past the limit: %s", context)
	}
	if !strings.Contains(context, "middle") || !strings.Contains(context, "new") {
		t.Fatalf("context did not keep recent messages: %s", context)
	}
}

func newContextTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.New(t.TempDir()+"/state.json", config.BotConfig{
		DefaultMode:       "funny",
		MinDelay:          3 * time.Minute,
		MaxRepliesPerHour: 10,
		ContextLimit:      10,
		PrivacyStoreText:  true,
	})
	if err != nil {
		t.Fatalf("storage init: %v", err)
	}
	return store
}
