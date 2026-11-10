package platform

import (
	"strings"
	"testing"
)

func occupied(o *Occupant) Entry {
	return Entry{
		Occupant:   func(string) *Occupant { return o },
		Release:    func(string) bool { return true },
		ModelNames: func() map[string]string { return map[string]string{"coder": "Qwen2.5-Coder-3B"} },
	}
}

func TestAPlatformWithoutTheSeamNeverConflicts(t *testing.T) {
	if c := (Entry{}).CheckOccupied("m", "u1", true); c != nil {
		t.Errorf("%+v", c)
	}
	if (Entry{}).ReleaseOccupant("m") {
		t.Error("nothing to release without the seam")
	}
}

func TestNothingLoadedIsNoConflict(t *testing.T) {
	if c := occupied(nil).CheckOccupied("coder", "u1", true); c != nil {
		t.Errorf("%+v", c)
	}
}

func TestALoadedModelThatNobodyUsesIsTold(t *testing.T) {
	e := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantIdle})
	c := e.CheckOccupied("coder", "u1", true)
	if c == nil {
		t.Fatal("no conflict")
	}
	want := "Phi-4-mini is loaded. Unload it before starting a conversation on Qwen2.5-Coder-3B."
	if c.Message != want || c.Code != "local_model_occupied" || c.State != OccupantIdle || c.LoadedModel != "phi" || c.LoadedModelName != "Phi-4-mini" || !c.CanRelease {
		t.Errorf("%+v", c)
	}
	if len(c.Conversations) != 0 || c.OthersUsing != 0 {
		t.Errorf("%+v", c)
	}
}

func TestALoadingModelCanBeReleasedToo(t *testing.T) {
	c := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantLoading}).CheckOccupied("coder", "u1", true)
	if c == nil || c.State != OccupantLoading || !c.CanRelease || !strings.HasPrefix(c.Message, "Phi-4-mini is loaded. Unload it") {
		t.Errorf("%+v", c)
	}
}

func TestAModelInUseNamesTheCallersOwnConversationAndCannotBeReleased(t *testing.T) {
	e := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantInUse, Conversations: []OccupantConversation{{ID: "c1", Title: "CV import", UserID: "u1"}}})
	c := e.CheckOccupied("coder", "u1", true)
	want := `Phi-4-mini is answering in the conversation "CV import". Stop that conversation first, then start this one on Qwen2.5-Coder-3B.`
	if c == nil || c.Message != want || c.CanRelease || c.State != OccupantInUse {
		t.Fatalf("%+v", c)
	}
	if len(c.Conversations) != 1 || c.Conversations[0] != (ConversationRef{ID: "c1", Title: "CV import"}) || c.OthersUsing != 0 {
		t.Errorf("%+v", c)
	}
}

func TestAnotherPersonsConversationIsCountedNeverNamed(t *testing.T) {
	e := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantInUse, Conversations: []OccupantConversation{
		{ID: "c2", Title: "secret title", UserID: "u2"},
	}})
	c := e.CheckOccupied("coder", "u1", true)
	if c == nil || c.OthersUsing != 1 || len(c.Conversations) != 0 {
		t.Fatalf("%+v", c)
	}
	if strings.Contains(c.Message, "secret") || !strings.Contains(c.Message, "in another conversation") {
		t.Errorf("%q", c.Message)
	}
	e = occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantInUse, Conversations: []OccupantConversation{
		{ID: "c1", Title: "mine", UserID: "u1"}, {ID: "c2", Title: "secret", UserID: "u2"}, {ID: "c3", Title: "secret too", UserID: "u3"},
	}})
	c = e.CheckOccupied("coder", "u1", true)
	if c == nil || c.OthersUsing != 2 || len(c.Conversations) != 1 || !strings.Contains(c.Message, `the conversation "mine" and 2 other conversations`) || strings.Contains(c.Message, "secret") {
		t.Errorf("%+v", c)
	}
}

func TestAModelInUseByARequestWithoutAConversationIsStillBusy(t *testing.T) {
	c := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantInUse}).CheckOccupied("coder", "u1", true)
	want := "Phi-4-mini is busy answering a request. Wait until it is done, then start this conversation on Qwen2.5-Coder-3B."
	if c == nil || c.Message != want || c.CanRelease {
		t.Errorf("%+v", c)
	}
}

func TestOnlyAPersonWhoMayUnloadIsOfferedTheUnload(t *testing.T) {
	e := occupied(&Occupant{ModelID: "phi", ModelName: "Phi-4-mini", State: OccupantIdle})
	if c := e.CheckOccupied("coder", "u1", false); c == nil || c.CanRelease {
		t.Errorf("%+v", c)
	}
	e.Release = nil
	if c := e.CheckOccupied("coder", "u1", true); c == nil || c.CanRelease {
		t.Errorf("a platform that cannot release: %+v", c)
	}
}

func TestNamesFallBackToTheIds(t *testing.T) {
	e := Entry{Occupant: func(string) *Occupant { return &Occupant{ModelID: "phi-id", State: OccupantIdle} }}
	c := e.CheckOccupied("coder-id", "u1", false)
	if c == nil || c.Message != "phi-id is loaded. Unload it before starting a conversation on coder-id." || c.LoadedModelName != "phi-id" {
		t.Errorf("%+v", c)
	}
}

func TestReleaseGoesToThePlatform(t *testing.T) {
	var got string
	e := Entry{Release: func(m string) bool { got = m; return false }}
	if e.ReleaseOccupant("coder") || got != "coder" {
		t.Errorf("%q", got)
	}
}
