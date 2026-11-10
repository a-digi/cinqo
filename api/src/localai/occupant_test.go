package localai

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/a-digi/cinqo/src/download"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/platform"
)

func ident(id string) string { return "name:" + id }

func TestNothingIsDisplacedWhenNothingIsLoadedOrTheSameModelIs(t *testing.T) {
	turns := []RunningTurn{{ConversationID: "c1", Title: "t", UserID: "u1", Model: "phi"}}
	for name, v := range map[string]agentView{
		"stopped":               {state: entity.EngineStopped, modelID: "phi"},
		"failed":                {state: entity.EngineFailed, modelID: "phi"},
		"no model id":           {state: entity.EngineReady},
		"the same one":          {state: entity.EngineReady, modelID: "coder", busy: true},
		"starting the same one": {state: entity.EngineStarting, modelID: "coder"},
	} {
		if o := occupantFor(v, "coder", turns, ident); o != nil {
			t.Errorf("%s: %+v", name, o)
		}
	}
}

func TestAnotherLoadedModelIsIdleLoadingOrInUse(t *testing.T) {
	cases := []struct {
		name  string
		v     agentView
		turns []RunningTurn
		want  string
		convs int
	}{
		{"idle", agentView{state: entity.EngineReady, modelID: "phi"}, nil, platform.OccupantIdle, 0},
		{"loading", agentView{state: entity.EngineStarting, modelID: "phi"}, nil, platform.OccupantLoading, 0},
		{"a turn is under way", agentView{state: entity.EngineReady, modelID: "phi"}, []RunningTurn{{ConversationID: "c1", Title: "CV", UserID: "u1", Model: "phi"}}, platform.OccupantInUse, 1},
		{"a turn between two requests (the program is idle)", agentView{state: entity.EngineReady, modelID: "phi", busy: false}, []RunningTurn{{ConversationID: "c1", Model: "phi"}}, platform.OccupantInUse, 1},
		{"a request without a conversation", agentView{state: entity.EngineReady, modelID: "phi", busy: true}, nil, platform.OccupantInUse, 0},
		{"a turn on a model that is still loading", agentView{state: entity.EngineStarting, modelID: "phi"}, []RunningTurn{{ConversationID: "c1", Model: "phi"}}, platform.OccupantInUse, 1},
		{"turns of other models do not count", agentView{state: entity.EngineReady, modelID: "phi"}, []RunningTurn{{ConversationID: "c9", Model: "coder"}, {ConversationID: "c8", Model: "other"}}, platform.OccupantIdle, 0},
		{"two conversations", agentView{state: entity.EngineReady, modelID: "phi"}, []RunningTurn{{ConversationID: "c1", Model: "phi"}, {ConversationID: "c2", Model: "phi"}}, platform.OccupantInUse, 2},
	}
	for _, c := range cases {
		o := occupantFor(c.v, "coder", c.turns, ident)
		if o == nil {
			t.Errorf("%s: no occupant", c.name)
			continue
		}
		if o.State != c.want || len(o.Conversations) != c.convs || o.ModelID != "phi" || o.ModelName != "name:phi" {
			t.Errorf("%s: %+v", c.name, o)
		}
	}
}

func TestTheConversationsKeepTheirIdTitleAndOwner(t *testing.T) {
	o := occupantFor(agentView{state: entity.EngineReady, modelID: "phi"}, "coder", []RunningTurn{{ConversationID: "c1", Title: "CV import", UserID: "u7", Model: "phi"}}, ident)
	if o == nil || len(o.Conversations) != 1 || o.Conversations[0] != (platform.OccupantConversation{ID: "c1", Title: "CV import", UserID: "u7"}) {
		t.Errorf("%+v", o)
	}
}

// --- through the platform entry and the real supervisor ---

func TestThePlatformEntrySetsTheSeamsAndIsSilentWithoutAnEngine(t *testing.T) {
	old := managed
	t.Cleanup(func() { managed = old })
	managed = nil
	e := Entry(openMainDB(t))
	if e.Occupant == nil || e.Release == nil {
		t.Fatal("the local platform sets both seams")
	}
	if o := e.Occupant("coder"); o != nil {
		t.Errorf("no engine, nothing loaded: %+v", o)
	}
	if !e.Release("coder") {
		t.Error("nothing to stop is a success")
	}
}

func TestTheSeamsFollowTheRealProgram(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	old := managed
	t.Cleanup(func() { managed = old })
	managed = &managedEngine{sup: s}
	t.Cleanup(func() { setRunningTurns(nil) })
	setRunningTurns(func() []RunningTurn {
		return []RunningTurn{{ConversationID: "c1", Title: "CV import", UserID: "u1", Model: "tiny"}}
	})
	e := Entry(openMainDB(t))

	if o := e.Occupant("other"); o != nil {
		t.Fatalf("not started: %+v", o)
	}
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0) // the model "tiny" is loaded and a request is under way
	if err != nil {
		t.Fatal(err)
	}
	if o := e.Occupant("tiny"); o != nil {
		t.Errorf("the same model: %+v", o)
	}
	o := e.Occupant("other")
	if o == nil || o.State != platform.OccupantInUse || o.ModelID != "tiny" || len(o.Conversations) != 1 || o.Conversations[0].Title != "CV import" {
		t.Fatalf("%+v", o)
	}
	if e.Release("other") {
		t.Fatal("a model in use is not released")
	}
	if st := s.Status(); st.State != EngineReady {
		t.Fatalf("a refused release stops nothing: %+v", st)
	}
	release() // the request is over; the conversation is still running a turn
	if o := e.Occupant("other"); o == nil || o.State != platform.OccupantInUse {
		t.Errorf("a running turn keeps it in use: %+v", o)
	}
	setRunningTurns(nil)
	if o := e.Occupant("other"); o == nil || o.State != platform.OccupantIdle {
		t.Errorf("%+v", o)
	}
	if !e.Release("other") {
		t.Fatal("an idle model is released")
	}
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("%+v", st)
	}
	if o := e.Occupant("other"); o != nil {
		t.Errorf("after the release: %+v", o)
	}
}

func TestRegisterTakesTheRunningTurnsFromTheApplication(t *testing.T) {
	oldManaged, oldEngine := managed, engine
	t.Cleanup(func() { managed, engine = oldManaged, oldEngine; setRunningTurns(nil) })
	want := []RunningTurn{{ConversationID: "c1", Title: "CV import", UserID: "u1", Model: "phi"}}
	// the platform registry is process-wide: another test may have registered it already, which Register reports after it has done its work
	err := Register(openMainDB(t), Deps{Downloads: download.New(download.Options{}), DataDir: t.TempDir(), RunningTurns: func() []RunningTurn { return want }})
	if err != nil && !errors.Is(err, platform.ErrInvalidEntry) {
		t.Fatal(err)
	}
	if got := currentRunningTurns(); len(got) != 1 || got[0] != want[0] {
		t.Errorf("%+v", got)
	}
	setRunningTurns(nil)
	if got := currentRunningTurns(); got != nil {
		t.Errorf("nobody known to use the model: %+v", got)
	}
}
