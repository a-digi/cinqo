package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
)

type fakeOccupant struct {
	o        *platform.Occupant
	released []string
	release  bool // what Release answers; the occupant is gone afterwards when true
}

func (f *fakeOccupant) entry() platform.Entry {
	return platform.Entry{
		Occupant: func(string) *platform.Occupant { return f.o },
		Release: func(m string) bool {
			f.released = append(f.released, m)
			if f.release {
				f.o = nil
			}
			return f.release
		},
	}
}

var idlePhi = &platform.Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: platform.OccupantIdle}

func inUsePhi(userID string) *platform.Occupant {
	return &platform.Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: platform.OccupantInUse,
		Conversations: []platform.OccupantConversation{{ID: "c1", Title: "CV import", UserID: userID}}}
}

func TestAStartWithNothingInTheWayGoesOn(t *testing.T) {
	f := &fakeOccupant{}
	if status, body := occupiedAnswer(f.entry(), "coder", "u1", nil, false); status != 0 || body != nil {
		t.Errorf("%d %+v", status, body)
	}
	if status, body := occupiedAnswer(platform.Entry{}, "m", "u1", nil, true); status != 0 || body != nil {
		t.Errorf("a platform without the seam: %d %+v", status, body)
	}
	if len(f.released) != 0 {
		t.Errorf("released %v", f.released)
	}
}

func TestAnotherLoadedModelIsA409WithTheCodeAndTheData(t *testing.T) {
	f := &fakeOccupant{o: idlePhi}
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{manageScope}, false)
	if status != http.StatusConflict || body == nil {
		t.Fatalf("%d %+v", status, body)
	}
	if !body.Error || body.Code != "local_model_occupied" || !strings.Contains(body.Message, "Phi-4-mini is loaded. Unload it") {
		t.Errorf("%+v", body)
	}
	d := body.Data
	if d.LoadedModel != "phi" || d.LoadedModelName != "Phi-4-mini" || d.State != "idle" || !d.CanRelease || d.OthersUsing != 0 || d.Conversations == nil || len(d.Conversations) != 0 {
		t.Errorf("%+v", d)
	}
	if len(f.released) != 0 {
		t.Errorf("without the field nothing is unloaded: %v", f.released)
	}
}

func TestTheAnswerOnTheWireIsTheEnvelopeWithCodeAndData(t *testing.T) {
	f := &fakeOccupant{o: inUsePhi("u1")}
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{superAdminScope}, false)
	rec := httptest.NewRecorder()
	writeOccupied(rec, status, body)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	var out struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
		Data    struct {
			State         string `json:"state"`
			CanRelease    bool   `json:"canRelease"`
			Conversations []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"conversations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Error || out.Code != "local_model_occupied" || out.Data.State != "in_use" || out.Data.CanRelease || len(out.Data.Conversations) != 1 || out.Data.Conversations[0].Title != "CV import" {
		t.Errorf("%s", rec.Body.String())
	}
	if !strings.Contains(out.Message, `the conversation "CV import"`) {
		t.Errorf("%q", out.Message)
	}
}

func TestOnlyThoseWhoMayManageThePlatformsAreOfferedTheUnload(t *testing.T) {
	for scopes, want := range map[string]bool{"": false, "cinqo:conversation:use": false, manageScope: true, superAdminScope: true} {
		f := &fakeOccupant{o: idlePhi}
		var sc []string
		if scopes != "" {
			sc = []string{"cinqo:conversation:use", scopes}
		}
		_, body := occupiedAnswer(f.entry(), "coder", "u1", sc, false)
		if body == nil || body.Data.CanRelease != want {
			t.Errorf("scope %q: %+v", scopes, body)
		}
	}
}

func TestTheReleaseFieldUnloadsAnIdleModelAndLetsTheStartGoOn(t *testing.T) {
	f := &fakeOccupant{o: idlePhi, release: true}
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{manageScope}, true)
	if status != 0 || body != nil {
		t.Fatalf("%d %+v", status, body)
	}
	if len(f.released) != 1 || f.released[0] != "coder" {
		t.Errorf("released %v", f.released)
	}
}

func TestTheReleaseFieldWithoutThePermissionIsA403AndUnloadsNothing(t *testing.T) {
	f := &fakeOccupant{o: idlePhi, release: true}
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{"cinqo:conversation:use"}, true)
	if status != http.StatusForbidden || body == nil || !strings.Contains(body.Message, "permission to manage the AI platforms") || body.Code != "forbidden" {
		t.Fatalf("%d %+v", status, body)
	}
	if len(f.released) != 0 || f.o == nil {
		t.Errorf("released %v", f.released)
	}
}

func TestAModelInUseIsNeverReleasedEvenWithTheFieldAndThePermission(t *testing.T) {
	f := &fakeOccupant{o: inUsePhi("u1"), release: true}
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{manageScope, superAdminScope}, true)
	if status != http.StatusConflict || body == nil || body.Data.State != "in_use" || body.Data.CanRelease {
		t.Fatalf("%d %+v", status, body)
	}
	if len(f.released) != 0 {
		t.Errorf("released %v", f.released)
	}
}

func TestARefusedReleaseSaysWhatIsThereNow(t *testing.T) {
	f := &fakeOccupant{o: idlePhi, release: false} // it became busy at that moment
	status, body := occupiedAnswer(f.entry(), "coder", "u1", []string{manageScope}, true)
	if status != http.StatusConflict || body == nil || body.Code != "local_model_occupied" {
		t.Fatalf("%d %+v", status, body)
	}
	if len(f.released) != 1 {
		t.Errorf("released %v", f.released)
	}
	// it went away by itself between the check and the release: the start goes on
	g := &fakeOccupant{o: idlePhi}
	e := g.entry()
	e.Release = func(string) bool { g.o = nil; return false }
	if status, body := occupiedAnswer(e, "coder", "u1", []string{manageScope}, true); status != 0 || body != nil {
		t.Errorf("%d %+v", status, body)
	}
}

// The three starts that load a model (create, send a message, retry a turn) must ask, once, before they go on; the handlers need a whole
// application around them to run, so this checks their source: each calls occupiedAnswer with the field of its own request.
func TestEveryStartAsksBeforeItGoesOn(t *testing.T) {
	for _, c := range []struct{ file, call, starts string }{
		{"create_handler.go", "occupiedAnswer(entry, model, userID, scopes, body.ReleaseLoadedModel)", "NewConversationPersistentRepo(db).Insert(c)"},
		{"send_message_handler.go", "occupiedAnswer(entry, conv.Model, userID, scopes, body.ReleaseLoadedModel)", "conversation.StartTurnRunWith("},
		{"retry_turn_handler.go", "occupiedAnswer(entry, conv.Model, userID, scopes, body.ReleaseLoadedModel)", "conversation.StartRetryRun("},
	} {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		if strings.Count(s, c.call) != 1 {
			t.Errorf("%s: expected exactly one %s", c.file, c.call)
		}
		i := strings.Index(s, "occupiedAnswer(")
		if i < 0 {
			t.Errorf("%s never asks", c.file)
		}
		if j := strings.Index(s, c.starts); j < 0 || i > j {
			t.Errorf("%s asks after it has started (or no longer starts with %s)", c.file, c.starts)
		}
		// answering must end the handler: the start must not go on after the answer
		if !strings.Contains(s, "writeOccupied(w, status, occupied)\n\t\t\treturn\n") && !strings.Contains(s, "writeOccupied(w, status, occupied)\n\t\treturn\n") {
			t.Errorf("%s does not answer", c.file)
		}
	}
}
