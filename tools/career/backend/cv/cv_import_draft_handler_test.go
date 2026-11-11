package cv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postDraft(handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/cv-import/draft", strings.NewReader(body)))
	return rec
}

func piece(t *testing.T, runID, section, field string, v any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"runId": runID, "section": section, field: v})
	return postDraft(CVImportDraftHandler, string(raw))
}

func finishVia(runID string) *httptest.ResponseRecorder {
	return postDraft(CVImportDraftFinishHandler, `{"runId":"`+runID+`"}`)
}

// The pieces sent over HTTP give exactly the proposal the tools' functions give.
func TestPiecesOverHTTPBecomeTheSameProposalAsOneCall(t *testing.T) {
	setup(t)
	id := newRun(t)
	p := okProposal()
	for section, v := range map[string]any{sectionProfile: p.Profile, sectionPersona: p.Persona, sectionPersonalDetails: p.PersonalDetails} {
		if rec := piece(t, id, section, "value", v); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", section, rec.Code, rec.Body.String())
		}
	}
	if rec := piece(t, id, "skills", "items", p.Skills); rec.Code != http.StatusOK {
		t.Fatalf("skills: %d %s", rec.Code, rec.Body.String())
	}
	rec := piece(t, id, "experience", "items", p.Experience)
	if rec.Code != http.StatusOK {
		t.Fatalf("experience: %d %s", rec.Code, rec.Body.String())
	}
	summary := decode(t, rec)
	if missing, _ := summary["stillMissing"].([]any); summary["stored"] != true || len(missing) != 0 {
		t.Errorf("summary: %v", summary)
	}
	if rec := finishVia(id); rec.Code != http.StatusOK || decode(t, rec)["ok"] != true {
		t.Fatalf("finish: %d %s", rec.Code, rec.Body.String())
	}
	whole := newRun(t)
	if err := StoreProposal(whole, okProposal()); err != nil {
		t.Fatal(err)
	}
	if got, want := withoutIDs(string(runOf(t, id).AIProposal)), withoutIDs(string(runOf(t, whole).AIProposal)); got != want {
		t.Errorf("HTTP pieces gave\n%s\nwant\n%s", got, want)
	}
}

// One entry may be sent as value and as items; education too.
func TestOneEntryMaySendAsValueOrItems(t *testing.T) {
	setup(t)
	id := newRun(t)
	job := okProposal().Experience[0]
	if rec := piece(t, id, "experience", "value", job); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := piece(t, id, "experience", "items", []ExperienceProposal{job, {Action: "insert", Company: "Beta", Title: "Lead", StartDate: "2023-02", Description: "Led."}}); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, piece(t, id, "education", "value", map[string]any{"action": "insert", "institution": "Uni", "degree": "BSc", "startDate": "2010", "endDate": "2013"}))["draft"].(map[string]any); got["experience"] != float64(2) || got["education"] != float64(1) {
		t.Errorf("draft: %v (the first job sent again replaces itself, so 2 jobs)", got)
	}
}

// A refused piece is a 400 with the same reason the tool gives, and what was stored before stays.
func TestAWrongPieceOverHTTPIsRefusedWithTheToolsReason(t *testing.T) {
	setup(t)
	id := newRun(t)
	if rec := piece(t, id, sectionProfile, "value", okProposal().Profile); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	bad := map[string]any{"action": "add", "company": "Acme", "title": "Dev", "startDate": "2020-05", "endDate": "2019-01", "description": "x"}
	rec := piece(t, id, "experience", "value", bad)
	_, want := AddDraftExperience(id, bad)
	if rec.Code != http.StatusBadRequest || want == nil || strings.TrimSpace(rec.Body.String()) != want.Error() {
		t.Errorf("%d %q, want 400 %v", rec.Code, rec.Body.String(), want)
	}
	if got := decode(t, piece(t, id, "skills", "items", okProposal().Skills))["draft"].(map[string]any); got["profile"] != true {
		t.Errorf("the stored profile was lost: %v", got)
	}
}

func TestAPieceSentAgainOverHTTPReplacesTheEarlierOne(t *testing.T) {
	setup(t)
	id := newRun(t)
	piece(t, id, sectionProfile, "value", map[string]any{"action": "insert", "name": "First"})
	piece(t, id, sectionProfile, "value", map[string]any{"action": "insert", "name": "Second"})
	var draft *string
	if err := dbDraft(id, &draft); err != nil || draft == nil || !strings.Contains(*draft, "Second") || strings.Contains(*draft, "First") {
		t.Errorf("draft: %v %v", draft, err)
	}
}

func TestFinishOverHTTPNamesWhatIsMissing(t *testing.T) {
	setup(t)
	id := newRun(t)
	piece(t, id, sectionProfile, "value", okProposal().Profile)
	rec := finishVia(id)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "still missing") || !strings.Contains(rec.Body.String(), "persona") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if r := runOf(t, id); string(r.AIProposal) != "null" {
		t.Errorf("an incomplete draft must store no proposal: %s", r.AIProposal)
	}
}

func TestTheRunMustBeNamedKnownAndGenerating(t *testing.T) {
	setup(t)
	good := okProposal().Profile
	for name, runID := range map[string]string{"empty": "", "not a uuid": "r1", "spaces": "  "} {
		if rec := piece(t, runID, sectionProfile, "value", good); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
		if rec := finishVia(runID); rec.Code != http.StatusBadRequest {
			t.Errorf("finish %s: %d", name, rec.Code)
		}
	}
	if rec := piece(t, "0b0f3c3e-0000-4000-8000-000000000000", sectionProfile, "value", good); rec.Code != http.StatusNotFound {
		t.Errorf("unknown run: %d %s", rec.Code, rec.Body.String())
	}
	if rec := finishVia("0b0f3c3e-0000-4000-8000-000000000000"); rec.Code != http.StatusNotFound {
		t.Errorf("finish unknown run: %d", rec.Code)
	}
	// a run that is no longer generating
	id := newRun(t)
	if _, err := finishCVImportRun(id, "failed", "stopped"); err != nil {
		t.Fatal(err)
	}
	if rec := piece(t, id, sectionProfile, "value", good); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no longer generating") {
		t.Errorf("ended run: %d %s", rec.Code, rec.Body.String())
	}
	if rec := finishVia(id); rec.Code != http.StatusConflict {
		t.Errorf("finish ended run: %d", rec.Code)
	}
}

func TestOnlyPostAndOnlyKnownSectionsAndJSON(t *testing.T) {
	setup(t)
	id := newRun(t)
	for _, h := range []http.HandlerFunc{CVImportDraftHandler, CVImportDraftFinishHandler} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(method, "/x", nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: %d", method, rec.Code)
			}
		}
		if rec := postDraft(h, "nope"); rec.Code != http.StatusBadRequest {
			t.Errorf("not json: %d", rec.Code)
		}
	}
	for _, section := range []string{"", "finish", "jobs", "Profile"} {
		if rec := piece(t, id, section, "value", map[string]any{"action": "skip"}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "section must be") {
			t.Errorf("section %q: %d %s", section, rec.Code, rec.Body.String())
		}
	}
	// nothing sent for a piece is a refusal from the tool's own check, not a crash
	if rec := postDraft(CVImportDraftHandler, `{"runId":"`+id+`","section":"skills"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no items: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postDraft(CVImportDraftHandler, `{"runId":"`+id+`","section":"skills","items":null,"value":null}`); rec.Code != http.StatusBadRequest {
		t.Errorf("null items: %d %s", rec.Code, rec.Body.String())
	}
	big := `{"runId":"` + id + `","section":"skills","items":[{"action":"insert","value":"` + strings.Repeat("a", maxDraftBody) + `"}]}`
	if rec := postDraft(CVImportDraftHandler, big); rec.Code != http.StatusBadRequest {
		t.Errorf("too big: %d", rec.Code)
	}
}

// When both are sent, the list wins over the single value.
func TestItemsWinOverValueWhenBothAreSent(t *testing.T) {
	setup(t)
	id := newRun(t)
	raw, _ := json.Marshal(map[string]any{
		"runId": id, "section": "experience",
		"items": []ExperienceProposal{{Action: "insert", Company: "FromItems", Title: "A", StartDate: "2020-01", Description: "x"}},
		"value": ExperienceProposal{Action: "insert", Company: "FromValue", Title: "B", StartDate: "2021-01", Description: "y"},
	})
	if rec := postDraft(CVImportDraftHandler, string(raw)); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var draft *string
	if err := dbDraft(id, &draft); err != nil || draft == nil || !strings.Contains(*draft, "FromItems") || strings.Contains(*draft, "FromValue") {
		t.Errorf("draft: %v %v", draft, err)
	}
}

// A request over the limit is refused even when what it holds would be fine.
func TestARequestOverTheLimitIsRefusedEvenIfItsPieceIsFine(t *testing.T) {
	setup(t)
	id := newRun(t)
	body := `{"runId":"` + id + `","section":"skills","items":[{"action":"insert","value":"Go"}],"pad":"` + strings.Repeat("a", maxDraftBody) + `"}`
	if rec := postDraft(CVImportDraftHandler, body); rec.Code != http.StatusBadRequest {
		t.Errorf("over the limit: %d", rec.Code)
	}
	small := `{"runId":"` + id + `","section":"skills","items":[{"action":"insert","value":"Go"}]}`
	if rec := postDraft(CVImportDraftHandler, small); rec.Code != http.StatusOK {
		t.Errorf("small: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postDraft(CVImportDraftFinishHandler, `{"runId":"`+id+`","pad":"`+strings.Repeat("a", maxDraftBody)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("finish over the limit: %d", rec.Code)
	}
}
