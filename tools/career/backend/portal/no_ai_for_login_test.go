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

// A link that needs a login needs a person to log in first (plan 183): only a manual start (the person's own Generate) is allowed; an automatic
// start is refused before any attempt row exists, so nothing is counted as a failed attempt.
func TestAnAutomaticAIRunCannotStartForALinkThatNeedsALogin(t *testing.T) {
	setupAttemptsDB(t, "plain", "gated")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET requires_login = 1 WHERE id = 'gated'`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{InstructionKindListing, InstructionKindJobDetail} {
		id, started, err := StartInstructionAttempt("gated", kind, InstructionTriggerAuto, "conv")
		if !errors.Is(err, ErrNoAIForLoginLink) || started || id != "" {
			t.Errorf("%s/auto: id=%q started=%v err=%v", kind, id, started, err)
		}
	}
	if n := attemptRows(t); n != 0 {
		t.Fatalf("a refused start left %d attempt row(s)", n)
	}
	// the person's own start works, for both kinds (the job-detail instructions follow in the same conversation)
	for _, kind := range []string{InstructionKindListing, InstructionKindJobDetail} {
		if _, started, err := StartInstructionAttempt("gated", kind, InstructionTriggerManual, "conv"); err != nil || !started {
			t.Errorf("%s/manual: started=%v err=%v", kind, started, err)
		}
	}
	// a plain link starts either way
	for _, trig := range []string{InstructionTriggerManual, InstructionTriggerAuto} {
		if _, started, err := StartInstructionAttempt("plain", InstructionKindListing, trig, "conv"); err != nil && !started {
			t.Errorf("plain/%s: started=%v err=%v", trig, started, err)
		}
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
