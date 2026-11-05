package jobmatcher

import "testing"

func idsOf(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.PersonaID
	}
	return out
}

// Each persona gets its own result, in the order given, and each is exactly what that persona alone would score.
func TestScoreEachGivesEveryPersonaItsOwnResultInOrder(t *testing.T) {
	c := corpusOf()
	got := ScoreEach(goJob(), []Persona{erzieher(), goDev()}, c, now)
	eqStrings(t, "personas", idsOf(got), "p-kita", "p-go")
	for _, p := range []Persona{erzieher(), goDev()} {
		alone := mustScore(t, goJob(), []Persona{p}, c)
		var mine Result
		for _, r := range got {
			if r.PersonaID == p.ID {
				mine = r
			}
		}
		if mine.Score != alone.Score || mine.PersonaName != alone.PersonaName || len(mine.MatchedSkills) != len(alone.MatchedSkills) {
			t.Errorf("%s: in a group %d (%s), alone %d (%s)", p.ID, mine.Score, mine.Describe(), alone.Score, alone.Describe())
		}
	}
	if got[1].Score <= got[0].Score {
		t.Errorf("the Go persona (%d) must score above the Erzieherin (%d) on a Go job", got[1].Score, got[0].Score)
	}
}

// A persona's own score does not depend on who else is in the list.
func TestScoreEachScoreOfAPersonaIsIndependentOfTheOthers(t *testing.T) {
	c := corpusOf()
	a := ScoreEach(goJob(), []Persona{goDev()}, c, now)
	b := ScoreEach(goJob(), []Persona{erzieher(), goDev(), erzieher()}, c, now)
	if len(a) != 1 || len(b) != 3 || a[0].Score != b[1].Score {
		t.Errorf("alone %v, in a group %v", a, idsOf(b))
	}
}

// A persona that cannot be judged has no entry; the others still do.
func TestScoreEachLeavesOutAPersonaThatCannotBeJudged(t *testing.T) {
	got := ScoreEach(goJob(), []Persona{{ID: "empty", Name: "Empty"}, goDev(), {ID: "empty2"}}, corpusOf(), now)
	eqStrings(t, "personas", idsOf(got), "p-go")
}

func TestScoreEachWithNoOneToJudgeIsEmptyNotAnError(t *testing.T) {
	c := corpusOf()
	if got := ScoreEach(goJob(), nil, c, now); len(got) != 0 {
		t.Errorf("no personas: %v", got)
	}
	if got := ScoreEach(goJob(), []Persona{{ID: "empty"}}, c, now); len(got) != 0 {
		t.Errorf("nobody can judge: %v", idsOf(got))
	}
}

// The best of ScoreEach is the answer of Score, ties included (the earlier persona, or the one with more matched skills).
func TestTheBestOfScoreEachIsWhatScoreReturns(t *testing.T) {
	c := corpusOf()
	a, b := goDev(), goDev()
	a.ID, b.ID = "a", "b"
	lean := goDev()
	lean.ID, lean.Skills = "lean", []string{"Go", "Docker", "PostgreSQL"}
	sets := [][]Persona{
		{erzieher(), goDev()}, {goDev(), erzieher()}, {a, b}, {b, a}, {lean, goDev()}, {goDev(), lean},
		{erzieher(), lean, goDev(), a, {ID: "empty"}},
	}
	for i, personas := range sets {
		for _, job := range []Job{goJob(), kitaJob()} {
			best, err := Score(job, personas, c, now)
			each := ScoreEach(job, personas, c, now)
			if err != nil || len(each) == 0 {
				t.Fatalf("set %d: %v %d", i, err, len(each))
			}
			pick := each[0]
			for _, r := range each[1:] {
				if betterThan(r, pick) {
					pick = r
				}
			}
			if pick.PersonaID != best.PersonaID || pick.Score != best.Score {
				t.Errorf("set %d %q: best of ScoreEach %s/%d, Score %s/%d", i, job.Title, pick.PersonaID, pick.Score, best.PersonaID, best.Score)
			}
		}
	}
}

func TestScoreEachScoresAreWithinRangeAndStamped(t *testing.T) {
	for _, r := range ScoreEach(goJob(), []Persona{goDev(), erzieher()}, corpusOf(), now) {
		if r.Score < 0 || r.Score > 100 || r.EngineVersion != EngineVersion || r.PersonaID == "" {
			t.Errorf("%+v", r)
		}
	}
}

// The best-persona rule, on written-out results: the higher score wins, a tie goes to more matched skills, and a full
// tie keeps the current best (the earlier persona).
func TestBetterThanRule(t *testing.T) {
	two := Result{Score: 70, MatchedSkills: []string{"a", "b"}}
	one := Result{Score: 70, MatchedSkills: []string{"a"}}
	cases := []struct {
		name             string
		challenger, best Result
		want             bool
	}{
		{"higher score", Result{Score: 71}, Result{Score: 70, MatchedSkills: []string{"a", "b", "c"}}, true},
		{"lower score", Result{Score: 69, MatchedSkills: []string{"a", "b", "c"}}, Result{Score: 70}, false},
		{"same score, more skills", two, one, true},
		{"same score, fewer skills", one, two, false},
		{"full tie keeps the best", two, two, false},
		{"full tie, no skills", Result{Score: 50}, Result{Score: 50}, false},
	}
	for _, c := range cases {
		if got := betterThan(c.challenger, c.best); got != c.want {
			t.Errorf("%s: betterThan = %v, want %v", c.name, got, c.want)
		}
	}
}
