package crawl

import (
	"context"
	"testing"
)

func TestPacingKeyGivesEveryCrawlerBeyondTheFirstALineOfItsOwn(t *testing.T) {
	ctx := context.Background()
	if got := PacingKey(ctx, "p1"); got != "p1" {
		t.Errorf("no slot: %q", got)
	}
	if got := PacingKey(WithPacingSlot(ctx, 0), "p1"); got != "p1" {
		t.Errorf("slot 0 keeps the platform's own key (shared with its other crawls): %q", got)
	}
	a, b := PacingKey(WithPacingSlot(ctx, 1), "p1"), PacingKey(WithPacingSlot(ctx, 2), "p1")
	if a == "p1" || b == "p1" || a == b {
		t.Errorf("slots 1 and 2 must have their own keys: %q %q", a, b)
	}
	if PacingKey(WithPacingSlot(ctx, 1), "p2") == a {
		t.Error("the key must still name the platform")
	}
}
