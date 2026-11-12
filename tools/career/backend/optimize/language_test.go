package optimize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type languageFixtures struct {
	Clear    map[string][]string `json:"clear"`
	Unplaced []string            `json:"unplaced"`
}

func loadFixtures(t *testing.T) languageFixtures {
	t.Helper()
	raw, err := os.ReadFile("testdata/language_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f languageFixtures
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEverySentenceOfTheFixturesIsPlacedInItsLanguage(t *testing.T) {
	f := loadFixtures(t)
	if len(f.Clear) != len(languageNames) {
		t.Fatalf("the fixtures have %d languages, the detector %d", len(f.Clear), len(languageNames))
	}
	for lang, texts := range f.Clear {
		if len(texts) < 3 {
			t.Errorf("%s: only %d fixtures", lang, len(texts))
		}
		for _, text := range texts {
			got, ok := detectLanguage(text)
			if !ok || got != lang {
				t.Errorf("%s text placed as %q (ok %v): %q", lang, got, ok, text)
			}
		}
	}
}

func TestWhatCannotBePlacedIsNotPlaced(t *testing.T) {
	for _, text := range loadFixtures(t).Unplaced {
		if got, ok := detectLanguage(text); ok {
			t.Errorf("%q was placed as %q", text, got)
		}
	}
}

func TestEveryLanguageHasWordsAndAName(t *testing.T) {
	for lang, words := range languageWords {
		if len(strings.Fields(words)) < 20 {
			t.Errorf("%s has only %d words", lang, len(strings.Fields(words)))
		}
		if languageNames[lang] == "" {
			t.Errorf("%s has no name", lang)
		}
		for _, w := range strings.Fields(words) {
			if w != strings.ToLower(w) {
				t.Errorf("%s: %q is not lower case", lang, w)
			}
		}
	}
}

func TestAMixedTextTakesTheLanguageOfMostOfItsWords(t *testing.T) {
	german := "Ich habe die Zahlungsplattform entwickelt, mit Go, Kubernetes und Terraform, und das Team hat die Auslieferung automatisiert und verbessert."
	if got, ok := detectLanguage(german); !ok || got != "de" {
		t.Errorf("German text with English technology names: %q %v", got, ok)
	}
	if _, ok := detectLanguage("der die das und ist nicht mit"); ok {
		t.Error("seven words are too few to place a text")
	}
	if _, ok := detectLanguage("der die das und ist nicht mit für"); !ok {
		t.Error("eight German function words are enough")
	}
	// Half of the words are German and half English: neither dominates.
	if got, ok := detectLanguage("the and with that for the and der die das und ist nicht mit für from this"); ok {
		t.Errorf("a text split in two languages was placed as %q", got)
	}
}

func TestCasingAndPunctuationDoNotMatter(t *testing.T) {
	if got, ok := detectLanguage("ICH HABE DIE PLATTFORM ENTWICKELT UND MIT DEM TEAM BETREUT, FÜR DIE KUNDEN UND SIE."); !ok || got != "de" {
		t.Errorf("%q %v", got, ok)
	}
}

const (
	germanText    = "Ich habe die Zahlungsplattform des Unternehmens entwickelt und mit dem Team über mehrere Jahre betreut und weiter verbessert."
	germanBetter  = "Ich habe die Zahlungsplattform des Unternehmens entwickelt und gemeinsam mit dem Team über mehrere Jahre betreut und kontinuierlich verbessert."
	englishBetter = "I built the payment platform of the company and worked with the team for several years to maintain and improve it."
)

func TestASuggestionInAnotherLanguageThanTheTextIsRefused(t *testing.T) {
	w := newWorld(t)
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, germanText, w.hobbyID)
	_, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, englishBetter, "")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "the text is German but your suggestion is English") || !strings.Contains(err.Error(), "never translate") {
		t.Fatalf("%v", err)
	}
	if n := len(rowsOf(t, w.run)); n != 0 {
		t.Errorf("%d suggestions were stored", n)
	}
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, germanBetter, ""); err != nil {
		t.Errorf("a suggestion in the language of the text is stored: %v", err)
	}
}

func TestEveryLanguageOfTheFixturesIsHeldToItsOwn(t *testing.T) {
	f := loadFixtures(t)
	w := newWorld(t)
	for lang, texts := range f.Clear {
		for other, others := range f.Clear {
			if other == lang {
				continue
			}
			exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, texts[0], w.hobbyID)
			_, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, others[1], "")
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "the text is "+languageNames[lang]+" but your suggestion is "+languageNames[other]) {
				t.Errorf("%s text, %s suggestion: %v", lang, other, err)
			}
		}
	}
}

func TestAReasonInAnotherLanguageThanTheTextIsRefusedToo(t *testing.T) {
	w := newWorld(t)
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, germanText, w.hobbyID)
	_, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, germanBetter, "The wording is clearer and more concrete, and every fact is kept as it was.")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "the text is German but your reason is English") {
		t.Fatalf("%v", err)
	}
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, germanBetter, "Die Formulierung ist klarer und die Fakten bleiben unverändert erhalten."); err != nil {
		t.Errorf("a reason in German: %v", err)
	}
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, germanBetter+" ", "Clearer."); err != nil {
		t.Errorf("a reason too short to place is not refused: %v", err)
	}
}

func TestNothingIsRefusedWhenTheLanguageCannotBeTold(t *testing.T) {
	w := newWorld(t)
	// the text is short: it cannot be placed, so an answer in any language is fine
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "plays in a chess club, and has done for years", ""); err != nil {
		t.Errorf("a short original: %v", err)
	}
	// a long German text and a short answer that cannot be placed
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, germanText, w.hobbyID)
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "Plattform entwickelt und betreut.", ""); err != nil {
		t.Errorf("a short suggestion: %v", err)
	}
	// a list of technologies in both
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, "Go, SQL, Docker, Kubernetes, AWS, Terraform, Linux, Git", w.hobbyID)
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "Go, SQL, Docker, Kubernetes, AWS, Terraform, Linux, Git, Bash", ""); err != nil {
		t.Errorf("a technology list: %v", err)
	}
}

func TestTheLanguageRuleOverTheEndpointAndTheToolDescription(t *testing.T) {
	w := newWorld(t)
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, germanText, w.hobbyID)
	body := fmt.Sprintf(`{"runId":%q,"kind":"hobby","itemId":%q,"field":"description","text":%q,"reason":""}`, w.run, w.hobbyID, englishBetter)
	r := call(SuggestionsHandler, "POST", "/optimize-runs/suggestions", body)
	if r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "the text is German but your suggestion is English") {
		t.Errorf("%d %s", r.Code, r.Body)
	}
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterSuggestOptimization(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, _ := cs.ListTools(ctx, nil)
	if d := list.Tools[0].Description; !strings.Contains(d, "SAME language as the text") || !strings.Contains(d, "never translate") || !strings.Contains(d, "German") {
		t.Errorf("description: %s", d)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "suggest_optimization", Arguments: map[string]any{"runId": w.run, "kind": "hobby", "itemId": w.hobbyID, "field": "description", "text": englishBetter, "reason": ""}})
	if err != nil || !res.IsError {
		t.Fatalf("the tool must refuse: %v %v", res, err)
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(tc.Text, "write the improved text in German") {
		t.Errorf("%v", res.Content)
	}
}

func TestTheThresholdsOfPlacingATextAreExact(t *testing.T) {
	// ten words, only "und" and "die" are German function words: two hits are not enough
	if got, ok := detectLanguage("Plattform Entwicklung Qualität Sicherheit Zahlungen Kunden Partner Team und die"); ok {
		t.Errorf("two hits placed it as %q", got)
	}
	// the same with a third one
	if got, ok := detectLanguage("Plattform Entwicklung Qualität Sicherheit Zahlungen Kunden Partner und die das"); !ok || got != "de" {
		t.Errorf("three hits: %q %v", got, ok)
	}
	// three hits among twenty-five words are 12 percent: too thin
	thin := "Plattform Entwicklung Qualität Sicherheit Zahlungen Kunden Partner Team Betrieb Wartung Planung Release Wachstum Analyse Prozess Struktur Beratung Konzept Vertrieb Handel Projekt Budget Ziel und die das"
	if got, ok := detectLanguage(thin); ok {
		t.Errorf("a thin share was placed as %q", got)
	}
	// and at twenty words (15 percent) it is enough
	if got, ok := detectLanguage("Plattform Entwicklung Qualität Sicherheit Zahlungen Kunden Partner Team Betrieb Wartung Planung Release Wachstum Analyse Prozess Struktur und die das"); !ok || got != "de" {
		t.Errorf("exactly 15 percent: %q %v", got, ok)
	}
}

func TestTheNextLanguageCountsWhateverOrderTheLanguagesAreVisited(t *testing.T) {
	// three English and four German function words: German does not dominate (4 < 1.5 x 3)
	mixed := "the and with und die das ist Plattform Team Kunden Qualität"
	for i := 0; i < 60; i++ {
		if got, ok := detectLanguage(mixed); ok {
			t.Fatalf("a text split between two languages was placed as %q", got)
		}
	}
}

func TestAShortOriginalAcceptsASuggestionInAnyLanguage(t *testing.T) {
	w := newWorld(t)
	// 'club player' cannot be placed: a long German suggestion is not refused
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, germanBetter, ""); err != nil {
		t.Errorf("%v", err)
	}
}
