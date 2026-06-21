package persona

import (
	"strings"
	"testing"
)

func TestPostProcessRewritesBoringOpening(t *testing.T) {
	text := PostProcess("Слушайте, давайте соберёмся и наведём порядок.")
	if strings.HasPrefix(strings.ToLower(text), "слушайте") {
		t.Fatalf("expected boring opening rewritten, got %q", text)
	}
	if strings.HasPrefix(strings.ToLower(text), "давайте") {
		t.Fatalf("expected directive opening rewritten, got %q", text)
	}
}
