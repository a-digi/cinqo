package recruiters

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"career-tool-backend/companies"
)

// fakeCore records the "career.recruiter.deleted" and "career.company.deleted" events it receives as "topic:id".
type fakeCore struct {
	mu     sync.Mutex
	events []string
}

func captureEvents(t *testing.T) *fakeCore {
	t.Helper()
	f := &fakeCore{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]string
		}
		_ = json.Unmarshal(raw, &ev)
		f.mu.Lock()
		switch ev.Topic {
		case "career.recruiter.deleted":
			f.events = append(f.events, "recruiter:"+ev.Payload["recruiter_id"])
		case "career.company.deleted":
			f.events = append(f.events, "company:"+ev.Payload["company_id"])
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return f
}

func (f *fakeCore) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string(nil), f.events...)
	f.events = nil
	return out
}

func TestDeletingARecruiterAnnouncesItOnceAndAnUnknownIdAnnouncesNothing(t *testing.T) {
	setup(t)
	id, err := CreateRecruiter([]string{"c1"}, "Ada", "L", "a@x")
	if err != nil {
		t.Fatal(err)
	}
	ev := captureEvents(t)
	if err := DeleteRecruiter(id); err != nil {
		t.Fatal(err)
	}
	if got, want := ev.take(), []string{"recruiter:" + id}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if err := DeleteRecruiter(id); err != nil { // already gone
		t.Fatal(err)
	}
	if err := DeleteRecruiter("nope"); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a delete that removed nothing announced %v", got)
	}
}

// Since plan 171 deleting a company only unlinks its recruiters: they stay, so nothing about them is announced.
func TestDeletingACompanyKeepsItsRecruitersAndAnnouncesNoRecruiter(t *testing.T) {
	setup(t)
	id, err := CreateRecruiter([]string{"c1"}, "Ada", "L", "a@x")
	if err != nil {
		t.Fatal(err)
	}
	ev := captureEvents(t)
	if err := companies.DeleteCompany("c1"); err != nil {
		t.Fatal(err)
	}
	for _, e := range ev.take() {
		if e == "recruiter:"+id {
			t.Errorf("a recruiter event was announced for a company delete: %s", e)
		}
	}
	if count(t, `SELECT COUNT(*) FROM recruiters`) != 1 {
		t.Error("the recruiter must stay when its company is deleted")
	}
}
