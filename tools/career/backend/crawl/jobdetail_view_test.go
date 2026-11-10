package crawl

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

const viewCopyID = "0123456789abcdef0123456789abcdef"

// withCopy is pickPageJSON with a page copy and numbered elements added, the way the Browser tool answers when it is asked for one (plan 249).
func withCopy(extra string) string {
	return strings.TrimSuffix(pickPageJSON, "}") + "," + extra + "}"
}

const goodCopy = `"snapshot":{"id":"` + viewCopyID + `","bytes":4321,"createdAt":"2026-11-10T22:00:00Z","truncated":true},
"viewElements":[{"n":1,"selector":"h1.job","unique":true,"tag":"h1","chars":15,"text":"Alpha Developer"},
{"n":2,"selector":"time","unique":false,"tag":"time","chars":10,"text":"28 Oct 2026","attribute":"datetime"}]`

func detailBody(id string, snapshot bool) string {
	if snapshot {
		return `{"portalLinkId":"` + id + `","snapshot":true}`
	}
	return `{"portalLinkId":"` + id + `"}`
}

func decodeCandidates(t *testing.T, out string) pickCandidatesResponse {
	t.Helper()
	var r pickCandidatesResponse
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestJobDetailCandidatesAsksForAPageCopyOnlyWhenTheScreenAsks(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(b map[string]any) (int, string) {
		if b["snapshot"] == true {
			return 200, withCopy(goodCopy)
		}
		return 200, pickPageJSON
	}
	code, with := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
	if code != 200 {
		t.Fatalf("%d %s", code, with)
	}
	code, without := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, false), true)
	if code != 200 {
		t.Fatalf("%d %s", code, without)
	}
	if len(f.detailBodies) != 2 || f.detailBodies[0]["snapshot"] != true {
		t.Fatalf("the Browser must be asked for a copy: %v", f.detailBodies)
	}
	if _, has := f.detailBodies[1]["snapshot"]; has {
		t.Errorf("no copy may be asked for unless the screen asks: %v", f.detailBodies[1])
	}
	a, b := decodeCandidates(t, with), decodeCandidates(t, without)
	if a.Snapshot == nil || a.Snapshot.ID != viewCopyID || a.Snapshot.Bytes != 4321 || !a.Snapshot.Truncated || a.SnapshotError != "" {
		t.Errorf("the copy must reach the screen: %+v %q", a.Snapshot, a.SnapshotError)
	}
	if len(a.ViewElements) != 2 || a.ViewElements[0].N != 1 || a.ViewElements[0].Selector != "h1.job" || !a.ViewElements[0].Unique || a.ViewElements[0].Tag != "h1" || a.ViewElements[0].Chars != 15 || a.ViewElements[0].Text != "Alpha Developer" ||
		a.ViewElements[1].Attribute != "datetime" || a.ViewElements[1].Unique {
		t.Errorf("the numbered elements must reach the screen as they came: %+v", a.ViewElements)
	}
	if b.Snapshot != nil || b.ViewElements != nil || strings.Contains(without, `"snapshot"`) || strings.Contains(without, `"viewElements"`) {
		t.Errorf("without a copy the answer is what it always was: %s", without)
	}
}

func TestAPageCopyChangesNothingOfTheSuggestions(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(b map[string]any) (int, string) {
		if b["snapshot"] == true {
			return 200, withCopy(goodCopy)
		}
		return 200, pickPageJSON
	}
	_, with := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
	_, without := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, false), true)
	a, b := decodeCandidates(t, with), decodeCandidates(t, without)
	for name, pair := range map[string][2]any{"ranked": {a.Ranked, b.Ranked}, "elements": {a.Elements, b.Elements}, "hints": {a.StructuredHints, b.StructuredHints}, "notices": {a.Notices, b.Notices}, "help": {a.DescriptionHelp, b.DescriptionHelp}, "samples": {a.Samples, b.Samples}, "current": {a.Current, b.Current}} {
		x, _ := json.Marshal(pair[0])
		y, _ := json.Marshal(pair[1])
		if string(x) != string(y) {
			t.Errorf("%s differs with a copy:\n%s\n%s", name, x, y)
		}
	}
	if a.URL != b.URL || a.Title != b.Title || a.JobCount != b.JobCount || a.PageTextChars != b.PageTextChars {
		t.Errorf("%+v vs %+v", a, b)
	}
}

func TestACopyThatCouldNotBeMadeIsExplainedAndTheListIsStillGood(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(map[string]any) (int, string) {
		return 200, withCopy(`"snapshotError":"the page is too large to show (30 MB)"`)
	}
	code, out := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
	r := decodeCandidates(t, out)
	if code != 200 || r.SnapshotError != "the page is too large to show (30 MB)" || r.Snapshot != nil || r.ViewElements != nil || len(r.Ranked["title"]) == 0 {
		t.Errorf("%d %s", code, out)
	}
}

func TestACopyThatIsNotUsableIsDropped(t *testing.T) {
	for name, extra := range map[string]string{
		"an id that is not a copy id":   `"snapshot":{"id":"../../etc/passwd","bytes":1,"createdAt":"x"},"viewElements":[{"n":1,"selector":"h1","unique":true,"tag":"h1","chars":1,"text":"a"}]`,
		"an upper-case id":              `"snapshot":{"id":"` + strings.ToUpper(viewCopyID) + `","bytes":1,"createdAt":"x"},"viewElements":[]`,
		"a short id":                    `"snapshot":{"id":"abc","bytes":1,"createdAt":"x"}`,
		"an id with something appended": `"snapshot":{"id":"` + viewCopyID + `?x=1","bytes":1,"createdAt":"x"}`,
		"an empty id":                   `"snapshot":{"id":"","bytes":1,"createdAt":"x"}`,
	} {
		id, _ := pickLink(t)
		f := newFakeCore(t, "katana")
		f.detailPage = func(map[string]any) (int, string) { return 200, withCopy(extra) }
		code, out := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
		r := decodeCandidates(t, out)
		if code != 200 || r.Snapshot != nil || r.ViewElements != nil || !strings.Contains(r.SnapshotError, "cannot use") {
			t.Errorf("%s: %d %s", name, code, out)
		}
	}
}

func TestElementsWithoutACopyAreDroppedAndTheListIsBounded(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(map[string]any) (int, string) {
		return 200, withCopy(`"viewElements":[{"n":1,"selector":"h1","unique":true,"tag":"h1","chars":1,"text":"a"}]`)
	}
	_, out := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
	if r := decodeCandidates(t, out); r.ViewElements != nil || r.Snapshot != nil {
		t.Errorf("elements that point into no copy must not be offered: %s", out)
	}
	var els []string
	for i := 1; i <= maxViewElements+25; i++ {
		els = append(els, `{"n":`+strconv.Itoa(i)+`,"selector":"p.x","unique":false,"tag":"p","chars":3,"text":"abc"}`)
	}
	f.detailPage = func(map[string]any) (int, string) {
		return 200, withCopy(`"snapshot":{"id":"` + viewCopyID + `","bytes":9,"createdAt":"x"},"viewElements":[` + strings.Join(els, ",") + `]`)
	}
	_, out = postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, true), true)
	r := decodeCandidates(t, out)
	if len(r.ViewElements) != maxViewElements || r.ViewElements[maxViewElements-1].N != maxViewElements || r.Snapshot == nil {
		t.Errorf("%d elements", len(r.ViewElements))
	}
}

func TestACopyNobodyAskedForIsNeverPassedOn(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(map[string]any) (int, string) { return 200, withCopy(goodCopy) } // the Browser makes one whatever is asked
	_, out := postPick(t, JobDetailCandidatesHandler, "/x", detailBody(id, false), true)
	if strings.Contains(out, `"snapshot"`) || strings.Contains(out, `"viewElements"`) || strings.Contains(out, viewCopyID) {
		t.Errorf("the screen would not know to delete it: %s", out)
	}
}

func TestTheDetailRequestRefusesOtherFieldsAndStillTakesTheSampleChoices(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(map[string]any) (int, string) { return 200, withCopy(goodCopy) }
	for _, body := range []string{
		`{"portalLinkId":"` + id + `","snapshot":true,"jobId":"` + ids[1] + `"}`,
		`{"portalLinkId":"` + id + `","snapshot":true,"url":"https://board.example/j/3"}`,
	} {
		if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 200 || !strings.Contains(out, viewCopyID) {
			t.Errorf("%s: %d %s", body, code, out)
		}
	}
	if code, _ := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","snapshot":true,"cookies":"a=b"}`, true); code != 400 {
		t.Errorf("other fields stay refused: %d", code)
	}
	if code, _ := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","snapshot":"yes"}`, true); code != 400 {
		t.Errorf("snapshot must be a boolean: %d", code)
	}
	if last := f.detailBodies[len(f.detailBodies)-1]; last["url"] != "https://board.example/j/3" || last["snapshot"] != true {
		t.Errorf("the Browser call: %v", last)
	}
}
