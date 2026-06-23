package reactions

import (
	"strings"
	"testing"
)

func TestCandidateForShortPainMessage(t *testing.T) {
	candidate, ok := CandidateFor("пиздец", "medium")
	if !ok {
		t.Fatalf("expected local reaction candidate")
	}
	if candidate.Topic != "pain_short" {
		t.Fatalf("expected pain_short topic, got %s", candidate.Topic)
	}
	if !strings.Contains(candidate.Text, "инфраструктурных") {
		t.Fatalf("unexpected reply: %s", candidate.Text)
	}
}

func TestCandidateForKeyboardLayoutGreeting(t *testing.T) {
	candidate, ok := CandidateFor("ghbdtn", "medium")
	if !ok {
		t.Fatalf("expected local reaction candidate")
	}
	if candidate.Topic != "hello_layout" {
		t.Fatalf("expected hello_layout topic, got %s", candidate.Topic)
	}
}

func TestCandidateRejectsLongMessage(t *testing.T) {
	_, ok := CandidateFor("понял, но теперь нужно расписать весь план деплоя и миграций", "medium")
	if ok {
		t.Fatalf("expected long message to skip local reactions")
	}
}

func TestCandidateForDismissiveLongMessage(t *testing.T) {
	candidate, ok := CandidateFor("а не пойти ли тебе нахуй? ты вообще полезное сделал?", "medium")
	if !ok {
		t.Fatalf("expected dismissive local reaction")
	}
	if candidate.Topic != "backoff_short" {
		t.Fatalf("expected backoff_short topic, got %s", candidate.Topic)
	}
	if !candidate.Backoff {
		t.Fatalf("expected dismissive local reaction to trigger backoff")
	}
	if strings.Contains(candidate.Text, "командир") {
		t.Fatalf("expected non-escalating reply, got %q", candidate.Text)
	}
}

func TestLocalReactionChanceForChattyModes(t *testing.T) {
	if got := chanceFor("high"); got < 0.60 {
		t.Fatalf("expected high chance >= 0.60, got %f", got)
	}
	if got := chanceFor("insane"); got < 0.90 {
		t.Fatalf("expected insane chance >= 0.90, got %f", got)
	}
}
