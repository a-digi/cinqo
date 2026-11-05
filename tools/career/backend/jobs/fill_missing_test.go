package jobs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
)

// events captures what the tool publishes to core.
type fillEvents struct {
	mu     sync.Mutex
	events []map[string]any
}

func captureFillEvents(t *testing.T) *fillEvents {
	t.Helper()
	c := &fillEvents{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var e map[string]any
		_ = json.Unmarshal(raw, &e)
		c.mu.Lock()
		c.events = append(c.events, e)
		c.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "svc")
	return c
}

func (c *fillEvents) topics() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, e := range c.events {
		out = append(out, e["topic"].(string))
	}
	return out
}

func (c *fillEvents) reset() {
	c.mu.Lock()
	c.events = nil
	c.mu.Unlock()
}

// fillSetup makes two links of one portal: l-links (method "links") and l-instr (method "instructions").
func fillSetup(t *testing.T) {
	t.Helper()
	byURLSetup(t)
	for _, q := range []string{
		`INSERT INTO portal_links (id, portal_id, url) VALUES ('l-instr', 'p1', 'https://board.example/other')`,
		`UPDATE portal_links SET crawl_method = 'links', link_rules = '{"host":"board.example","include":["board.example/j/*"]}' WHERE id = 'l1'`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func fieldsOf(t *testing.T, id string) (title, company, location, postedAt string) {
	t.Helper()
	var c, l, p *string
	if err := db.JobsDB.QueryRow(`SELECT title, company, location, posted_at FROM jobs WHERE id = ?`, id).Scan(&title, &c, &l, &p); err != nil {
		t.Fatal(err)
	}
	str := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	return title, str(c), str(l), str(p)
}

func newLinkJob(t *testing.T, link, url, title string) string {
	t.Helper()
	id, created, _, err := SavePortalJobByURL(NewCriteria(), link, url, title, "", "", "", "")
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	return id
}

func TestAJobOfTheLinkCrawlerIsCompletedFromItsDetailPage(t *testing.T) {
	fillSetup(t)
	ev := captureFillEvents(t)
	id := newLinkJob(t, "l1", "https://board.example/j/1", "Backend Developer")
	ev.reset()

	filled, err := FillMissingJobFields(id, MissingFields{Company: "  2RM   Lab ", Location: "Tirana,\n Albania", PostedAt: "23-11-2026"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filled, []string{"company", "location", "postedAt"}) {
		t.Errorf("filled %v", filled)
	}
	title, company, location, postedAt := fieldsOf(t, id)
	if title != "Backend Developer" || company != "2RM Lab" || location != "Tirana, Albania" || postedAt != "23-11-2026" {
		t.Errorf("%q %q %q %q", title, company, location, postedAt)
	}
	// a filled company reaches the company linking exactly like a crawl that came with its company; location and date announce nothing
	if got := ev.topics(); !reflect.DeepEqual(got, []string{"career.job.created"}) {
		t.Errorf("events %v", got)
	}
	ev.mu.Lock()
	payload, _ := json.Marshal(ev.events[0]["payload"])
	ev.mu.Unlock()
	if !strings.Contains(string(payload), `"company":"2RM Lab"`) || !strings.Contains(string(payload), id) {
		t.Errorf("payload %s", payload)
	}
	// a second time changes nothing: every field is filled now
	ev.reset()
	filled, err = FillMissingJobFields(id, MissingFields{Company: "Other", Location: "Elsewhere", PostedAt: "01-01-2020"})
	if err != nil || len(filled) != 0 {
		t.Errorf("second fill: %v %v", filled, err)
	}
	if _, c2, l2, p2 := fieldsOf(t, id); c2 != "2RM Lab" || l2 != "Tirana, Albania" || p2 != "23-11-2026" {
		t.Errorf("values were replaced: %q %q %q", c2, l2, p2)
	}
	if len(ev.topics()) != 0 {
		t.Errorf("events after a no-op: %v", ev.topics())
	}
}

func TestOnlyTheEmptyFieldsAreFilled(t *testing.T) {
	fillSetup(t)
	captureFillEvents(t)
	id := newLinkJob(t, "l1", "https://board.example/j/2", "Frontend Developer")
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET location = 'Berlin' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	filled, err := FillMissingJobFields(id, MissingFields{Company: "Acme", Location: "Tirana", PostedAt: ""})
	if err != nil || !reflect.DeepEqual(filled, []string{"company"}) {
		t.Fatalf("%v %v", filled, err)
	}
	if _, company, location, postedAt := fieldsOf(t, id); company != "Acme" || location != "Berlin" || postedAt != "" {
		t.Errorf("%q %q %q", company, location, postedAt)
	}
	// a company of only spaces counts as empty
	id2 := newLinkJob(t, "l1", "https://board.example/j/3", "Data Engineer")
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET company = '   ' WHERE id = ?`, id2); err != nil {
		t.Fatal(err)
	}
	if filled, _ := FillMissingJobFields(id2, MissingFields{Company: "Acme"}); !reflect.DeepEqual(filled, []string{"company"}) {
		t.Errorf("blank company: %v", filled)
	}
}

func TestOnlyJobsOfTheLinkCrawlerAreTouched(t *testing.T) {
	fillSetup(t)
	ev := captureFillEvents(t)
	f := MissingFields{Company: "Acme", Location: "Tirana", PostedAt: "01-01-2026"}

	// a job of a link on the instructions method
	other, _, _, err := SavePortalJobWith(NewCriteria(), "l-instr", "https://board.example/other/1", "Designer", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// a job somebody added by hand to the link-crawler link
	manual := newLinkJob(t, "l1", "https://board.example/j/9", "Product Manager")
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET source = 'manual' WHERE id = ?`, manual); err != nil {
		t.Fatal(err)
	}
	// a job that belongs to no link
	orphan := newLinkJob(t, "l1", "https://board.example/j/10", "Cook")
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET portal_link_id = NULL WHERE id = ?`, orphan); err != nil {
		t.Fatal(err)
	}
	ev.reset()
	for name, id := range map[string]string{"instructions link": other, "manual job": manual, "no link": orphan} {
		filled, err := FillMissingJobFields(id, f)
		if err != nil || len(filled) != 0 {
			t.Errorf("%s: filled %v %v", name, filled, err)
		}
		if _, company, location, postedAt := fieldsOf(t, id); company != "" || location != "" || postedAt != "" {
			t.Errorf("%s was changed: %q %q %q", name, company, location, postedAt)
		}
	}
	if len(ev.topics()) != 0 {
		t.Errorf("events %v", ev.topics())
	}
	// a link switched back to the instructions method stops receiving fills
	back := newLinkJob(t, "l1", "https://board.example/j/11", "Tester")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'instructions' WHERE id = 'l1'`); err != nil {
		t.Fatal(err)
	}
	if filled, _ := FillMissingJobFields(back, f); len(filled) != 0 {
		t.Errorf("a job of a link that is no longer on the link crawler was filled: %v", filled)
	}
}

func TestAValueThatIsNotAFieldIsIgnored(t *testing.T) {
	fillSetup(t)
	ev := captureFillEvents(t)
	id := newLinkJob(t, "l1", "https://board.example/j/4", "QA Engineer")
	ev.reset()
	long := strings.Repeat("a", maxFilledCompanyRunes+1)
	filled, err := FillMissingJobFields(id, MissingFields{Company: long, Location: strings.Repeat("b", maxFilledLocationRunes+1), PostedAt: strings.Repeat("1", maxFilledPostedAtRunes+1)})
	if err != nil || len(filled) != 0 {
		t.Fatalf("%v %v", filled, err)
	}
	if _, company, location, postedAt := fieldsOf(t, id); company != "" || location != "" || postedAt != "" {
		t.Errorf("a block of text became a field: %q %q %q", company, location, postedAt)
	}
	// exactly at the limits is accepted
	filled, _ = FillMissingJobFields(id, MissingFields{Company: strings.Repeat("a", maxFilledCompanyRunes), Location: strings.Repeat("ü", maxFilledLocationRunes), PostedAt: strings.Repeat("1", maxFilledPostedAtRunes)})
	if len(filled) != 3 {
		t.Errorf("at the limits: %v", filled)
	}
	// nothing, or only blanks, fills nothing
	id2 := newLinkJob(t, "l1", "https://board.example/j/5", "Site Reliability Engineer")
	if filled, err := FillMissingJobFields(id2, MissingFields{Company: "  ", Location: "\n\t", PostedAt: ""}); err != nil || len(filled) != 0 {
		t.Errorf("%v %v", filled, err)
	}
}

func TestFillingAnUnknownJobIsAnError(t *testing.T) {
	fillSetup(t)
	if _, err := FillMissingJobFields("nobody", MissingFields{Company: "x"}); err != ErrUnknownJob {
		t.Errorf("%v", err)
	}
}
