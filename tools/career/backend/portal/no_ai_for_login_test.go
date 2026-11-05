package portal

import (
	"errors"
	"testing"

	"career-tool-backend/db"
)

func attemptRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The AI cannot write instructions for a link that needs a login (its tools read the logged-out page): the start is refused before any
// attempt row exists, so nothing is counted as a failed attempt ("Probably can't be crawled").
func TestAnAIRunCannotStartForALinkThatNeedsALogin(t *testing.T) {
	setupAttemptsDB(t, "plain", "gated")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET requires_login = 1 WHERE id = 'gated'`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{InstructionKindListing, InstructionKindJobDetail} {
		for _, trigger := range []string{InstructionTriggerManual, InstructionTriggerAuto} {
			id, started, err := StartInstructionAttempt("gated", kind, trigger, "conv")
			if !errors.Is(err, ErrNoAIForLoginLink) || started || id != "" {
				t.Errorf("%s/%s: id=%q started=%v err=%v", kind, trigger, id, started, err)
			}
		}
	}
	if n := attemptRows(t); n != 0 {
		t.Fatalf("a refused start left %d attempt row(s)", n)
	}
	// a plain link still starts
	if _, started, err := StartInstructionAttempt("plain", InstructionKindListing, InstructionTriggerManual, "conv"); err != nil || !started {
		t.Fatalf("plain: started=%v err=%v", started, err)
	}
	// after the login is taken away it starts too
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET requires_login = 0 WHERE id = 'gated'`); err != nil {
		t.Fatal(err)
	}
	if _, started, err := StartInstructionAttempt("gated", InstructionKindListing, InstructionTriggerManual, "conv"); err != nil || !started {
		t.Fatalf("after the login was removed: started=%v err=%v", started, err)
	}
}

func TestTheOtherStartChecksComeFirst(t *testing.T) {
	setupAttemptsDB(t, "gated")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET requires_login = 1 WHERE id = 'gated'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := StartInstructionAttempt("gated", "nonsense", InstructionTriggerManual, ""); !errors.Is(err, ErrInvalidInstructionKind) {
		t.Errorf("kind: %v", err)
	}
	if _, _, err := StartInstructionAttempt("gated", InstructionKindListing, "nonsense", ""); !errors.Is(err, ErrInvalidInstructionTrigger) {
		t.Errorf("trigger: %v", err)
	}
	if _, _, err := StartInstructionAttempt("nobody", InstructionKindListing, InstructionTriggerManual, ""); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
}
