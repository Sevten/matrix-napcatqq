package msgconv

import (
	"testing"

	"github.com/sevten/matrix-napcatqq/pkg/onebot"
)

func TestToPlainTextSpecialSegments(t *testing.T) {
	got := ToPlainText([]onebot.Segment{
		onebot.NewSegment("mface", map[string]any{"summary": "party"}),
		onebot.NewSegment("dice", map[string]any{"result": "6"}),
		onebot.NewSegment("rps", map[string]any{"result": "rock"}),
		onebot.NewSegment("poke", map[string]any{"qq": "12345"}),
	})
	want := "[party][Dice: 6][Rock-paper-scissors: rock][Poke: 12345]"
	if got != want {
		t.Fatalf("unexpected plain text: got %q want %q", got, want)
	}
}
