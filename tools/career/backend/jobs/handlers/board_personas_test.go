package jobshandlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/match/personamatch"
	"career-tool-backend/persona"
)

func boardOf(t *testing.T, profileID, query string) jobboard.Board {
	t.Helper()
	rec := handlertest.BoardCall(t, JobBoardHandler, "GET", "/job-board?profileId="+profileID+query, "")
	handlertest.WantStatus(t, rec, 200)
	var b jobboard.Board
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return b
}

func cardOf(t *testing.T, b jobboard.Board, jobID string) jobboard.Card {
	t.Helper()
	for _, c := range b.Columns {
		for _, card := range c.Jobs {
			if card.JobID == jobID {
				return card
			}
		}
	}
	t.Fatalf("job %s is not on the board", jobID)
	return jobboard.Card{}
}

func summary(ps []jobboard.PersonaScore) []string {
	out := []string{}
	for _, p := range ps {
		mark := ""
		if p.Best {
			mark = "*"
		}
		out = append(out, fmt.Sprintf("%s:%d%s", p.PersonaName, p.Score, mark))
	}
	return out
}

func TestTheBoardCardsShowThisProfilesPersonaScoresBestFirst(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	b, _ := persona.CreatePersona(p1, "Beta", "")
	c, _ := persona.CreatePersona(p1, "Gamma", "")
	x, _ := persona.CreatePersona(p2, "Other profile", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	handlertest.MustSet(t, p1, "j2", jobboard.AppliedID)
	if err := personamatch.Replace("j1", []personamatch.Row{
		{PersonaID: a, ProfileID: p1, Score: 40}, {PersonaID: b, ProfileID: p1, Score: 85}, {PersonaID: c, ProfileID: p1, Score: 60},
		{PersonaID: x, ProfileID: p2, Score: 99}, // another profile's persona: never on this board
	}); err != nil {
		t.Fatal(err)
	}
	// j2 has scores of the other profile only

	if err := personamatch.Replace("j2", []personamatch.Row{{PersonaID: x, ProfileID: p2, Score: 70}}); err != nil {
		t.Fatal(err)
	}
	board := boardOf(t, p1, "")
	card := cardOf(t, board, "j1")
	if got := summary(card.Personas); !reflect.DeepEqual(got, []string{"Beta:85*", "Gamma:60", "Alpha:40"}) {
		t.Errorf("j1 personas = %v", got)
	}
	if card.Personas[0].PersonaID != b {
		t.Error("the persona id of the best one is missing")
	}
	if p := cardOf(t, board, "j2").Personas; p != nil {
		t.Errorf("j2 has only another profile's scores, its card must have none: %v", p)
	}
	// the other profile's board is not affected by Ada's personas
	handlertest.MustSet(t, p2, "j1", jobboard.NotReviewedID)
	if got := summary(cardOf(t, boardOf(t, p2, ""), "j1").Personas); !reflect.DeepEqual(got, []string{"Other profile:99*"}) {
		t.Errorf("the other profile's card = %v", got)
	}
}

func TestACardWithoutScoresHasNoPersonasField(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	rec := handlertest.BoardCall(t, JobBoardHandler, "GET", "/job-board?profileId="+p1, "")
	handlertest.WantStatus(t, rec, 200)
	var raw struct {
		Columns []struct {
			Jobs []map[string]any `json:"jobs"`
		} `json:"columns"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, c := range raw.Columns {
		for _, j := range c.Jobs {
			if _, has := j["personas"]; has {
				t.Errorf("a card without scores must not carry a personas field: %v", j)
			}
		}
	}
}

// The persona's name is read as it is now, and the scores of a persona that no longer exists are not shown.
func TestNamesAreCurrentAndADeletedPersonasScoresAreSkipped(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Old name", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 50}, {PersonaID: "ghost", ProfileID: p1, Score: 90}})
	if got := summary(cardOf(t, boardOf(t, p1, ""), "j1").Personas); !reflect.DeepEqual(got, []string{"Old name:50*"}) {
		t.Errorf("with a ghost row: %v", got)
	}
	if err := persona.UpdatePersona(a, handlertest.StrPtr("New name"), nil); err != nil {
		t.Fatal(err)
	}
	if got := summary(cardOf(t, boardOf(t, p1, ""), "j1").Personas); !reflect.DeepEqual(got, []string{"New name:50*"}) {
		t.Errorf("after a rename: %v", got)
	}
}

// Equal scores come in the order the personas were created, so the best one is never a matter of chance.
func TestEqualScoresFollowTheOrderThePersonasWereCreated(t *testing.T) {
	personas := []boardPersona{{"first", "First"}, {"second", "Second"}, {"third", "Third"}}
	rows := []personamatch.Row{{PersonaID: "third", Score: 70}, {PersonaID: "first", Score: 70}, {PersonaID: "second", Score: 70}}
	got := personaScoresOf(rows, personas)
	if s := summary(got); !reflect.DeepEqual(s, []string{"First:70*", "Second:70", "Third:70"}) {
		t.Errorf("order = %v", s)
	}
	rows2 := []personamatch.Row{{PersonaID: "first", Score: 70}, {PersonaID: "second", Score: 71}, {PersonaID: "third", Score: 70}}
	if s := summary(personaScoresOf(rows2, personas)); !reflect.DeepEqual(s, []string{"Second:71*", "First:70", "Third:70"}) {
		t.Errorf("a higher score first, then the creation order: %v", s)
	}
}

func TestPersonaScoresOfNothingIsNil(t *testing.T) {
	if got := personaScoresOf(nil, []boardPersona{{"a", "A"}}); got != nil {
		t.Errorf("no rows: %v", got)
	}
	if got := personaScoresOf([]personamatch.Row{{PersonaID: "gone", Score: 5}}, []boardPersona{{"a", "A"}}); got != nil {
		t.Errorf("only a deleted persona: %v", got)
	}
	if got := personaScoresOf([]personamatch.Row{{PersonaID: "a", Score: 5}}, nil); got != nil {
		t.Errorf("no personas: %v", got)
	}
}

// Only the first of the list is the best, and a single persona is the best of itself.
func TestOnlyTheFirstIsMarkedBest(t *testing.T) {
	got := personaScoresOf([]personamatch.Row{{PersonaID: "a", Score: 10}, {PersonaID: "b", Score: 20}, {PersonaID: "c", Score: 30}}, []boardPersona{{"a", "A"}, {"b", "B"}, {"c", "C"}})
	best := 0
	for _, p := range got {
		if p.Best {
			best++
		}
	}
	if best != 1 || !got[0].Best {
		t.Errorf("best marks = %d, first = %v", best, got[0].Best)
	}
	if one := personaScoresOf([]personamatch.Row{{PersonaID: "a", Score: 0}}, []boardPersona{{"a", "A"}}); len(one) != 1 || !one[0].Best {
		t.Errorf("a single persona with 0%%: %v", one)
	}
}

// The order of the cards (best match first) and everything else on the board stay as they were.
func TestTheBoardsOtherFieldsAndOrderAreUnchanged(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	handlertest.MustSet(t, p1, "j2", jobboard.NotReviewedID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 10}})
	_ = personamatch.Replace("j2", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 90}})
	plain, err := jobboard.GetBoardSorted(p1, "moved")
	if err != nil {
		t.Fatal(err)
	}
	withScores := boardOf(t, p1, "")
	if len(withScores.Columns) != len(plain.Columns) {
		t.Fatalf("columns %d vs %d", len(withScores.Columns), len(plain.Columns))
	}
	for i, col := range plain.Columns {
		if col.ID != withScores.Columns[i].ID || len(col.Jobs) != len(withScores.Columns[i].Jobs) {
			t.Errorf("column %s changed", col.Name)
		}
		for j, card := range col.Jobs {
			got := withScores.Columns[i].Jobs[j]
			if got.JobID != card.JobID || got.Title != card.Title || got.UpdatedAt != card.UpdatedAt {
				t.Errorf("card %s changed or moved: %+v vs %+v", card.JobID, got, card)
			}
		}
	}
	// sorting by match is the same call as before
	rec := handlertest.BoardCall(t, JobBoardHandler, "GET", "/job-board?profileId="+p1+"&sort=match", "")
	handlertest.WantStatus(t, rec, 200)
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobBoardHandler, "GET", "/job-board?profileId="+p1+"&sort=bogus", ""), 400)
}

// A side view must never break the board: when the personas cannot be read the board is answered as before.
func TestTheBoardStillWorksWhenThePersonasCannotBeRead(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 10}})
	old := boardPersonas
	boardPersonas = func(string) ([]boardPersona, error) { return nil, errors.New("career.db gone") }
	t.Cleanup(func() { boardPersonas = old })
	board := boardOf(t, p1, "")
	card := cardOf(t, board, "j1")
	if card.Personas != nil || card.Title == "" {
		t.Errorf("card = %+v", card)
	}
}

// A board of a profile whose personas have no scores yet (or none at all) is the same as before.
func TestABoardWithoutAnyScoresIsAsBefore(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	board := boardOf(t, p1, "")
	if p := cardOf(t, board, "j1").Personas; p != nil {
		t.Errorf("personas = %v", p)
	}
	empty := boardOf(t, p1, "")
	if len(empty.Columns) != 4 {
		t.Errorf("columns = %d", len(empty.Columns))
	}
}

// Equal scores follow the order the personas were CREATED (their created time), not the order of their ids.
func TestEqualScoresFollowCreatedTimeNotIdOrder(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Created later", "")
	b, _ := persona.CreatePersona(p1, "Created earlier", "")
	// make the persona with the smaller id the later one, so an id order would give the opposite answer
	small, large := a, b
	if large < small {
		small, large = large, small
	}
	for id, at := range map[string]string{small: "2026-06-02 10:00:00", large: "2026-06-01 10:00:00"} {
		if _, err := db.CareerDB.Exec(`UPDATE personas SET created_at = ? WHERE id = ?`, at, id); err != nil {
			t.Fatal(err)
		}
	}
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: small, ProfileID: p1, Score: 60}, {PersonaID: large, ProfileID: p1, Score: 60}})
	got := cardOf(t, boardOf(t, p1, ""), "j1").Personas
	if len(got) != 2 || got[0].PersonaID != large || !got[0].Best {
		t.Errorf("the earlier created persona must be first and best: %v (earlier id %s, later id %s)", summary(got), small, large)
	}
}

// Cards in different columns each get their own scores.
func TestEveryColumnsCardsGetTheirScores(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	custom, err := jobboard.CreateStatus(p1, "To check")
	if err != nil {
		t.Fatal(err)
	}
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	handlertest.MustSet(t, p1, "j2", custom.ID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 11}})
	_ = personamatch.Replace("j2", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 22}})
	board := boardOf(t, p1, "")
	if got := summary(cardOf(t, board, "j1").Personas); !reflect.DeepEqual(got, []string{"Alpha:11*"}) {
		t.Errorf("j1 = %v", got)
	}
	if got := summary(cardOf(t, board, "j2").Personas); !reflect.DeepEqual(got, []string{"Alpha:22*"}) {
		t.Errorf("j2 (in a later, custom column) = %v", got)
	}
}

func TestTheBoardStillWorksWhenTheScoresCannotBeRead(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	_, _ = persona.CreatePersona(p1, "Alpha", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	old := boardScores
	boardScores = func(string, []string) (map[string][]personamatch.Row, error) { return nil, errors.New("jobs.db gone") }
	t.Cleanup(func() { boardScores = old })
	card := cardOf(t, boardOf(t, p1, ""), "j1")
	if card.Personas != nil || card.Title == "" {
		t.Errorf("card = %+v", card)
	}
}

// In the JSON only the best persona carries "best"; the others have no such field, and a card without scores no "personas".
func TestOnlyTheBestPersonaCarriesTheBestFieldInTheJSON(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	b, _ := persona.CreatePersona(p1, "Beta", "")
	handlertest.MustSet(t, p1, "j1", jobboard.NotReviewedID)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 30}, {PersonaID: b, ProfileID: p1, Score: 80}})
	rec := handlertest.BoardCall(t, JobBoardHandler, "GET", "/job-board?profileId="+p1, "")
	var raw struct {
		Columns []struct {
			Jobs []struct {
				Personas []map[string]any `json:"personas"`
			} `json:"jobs"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, c := range raw.Columns {
		for _, j := range c.Jobs {
			if len(j.Personas) != 2 {
				t.Fatalf("personas = %v", j.Personas)
			}
			if j.Personas[0]["best"] != true {
				t.Errorf("the first must say best: %v", j.Personas[0])
			}
			if _, has := j.Personas[1]["best"]; has {
				t.Errorf("a persona that is not the best must not carry the field: %v", j.Personas[1])
			}
			for _, k := range []string{"personaId", "personaName", "score"} {
				if _, has := j.Personas[0][k]; !has {
					t.Errorf("missing %s: %v", k, j.Personas[0])
				}
			}
		}
	}
}
