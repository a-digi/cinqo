package persistent_test

import (
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

func TestRunningTurnsOnAPlatformNameTheirConversationOwnerAndModel(t *testing.T) {
	db := openConversationDB(t)
	convs := conversation_persistent.NewConversationPersistentRepo(db)
	runs := conversation_persistent.NewTurnRunPersistentRepo(db)
	add := func(id, user, title, platform, model string) {
		t.Helper()
		c := newConversation(id, "direct")
		c.UserID, c.Title, c.PlatformID, c.Model = user, title, platform, model
		if err := convs.Insert(c); err != nil {
			t.Fatal(err)
		}
	}
	run := func(id, convID, status string) {
		t.Helper()
		// Insert always starts a run as "running"; a finished one is ended afterwards.
		if err := runs.Insert(&conversation_entity.TurnRun{ID: id, ConversationID: convID, UserContent: "hi", StartedAt: "2026-11-10T10:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if status != "running" {
			if err := runs.SetTerminalStatus(id, status, "2026-11-10T10:01:00Z"); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("c1", "u1", "CV import", "local", "phi")
	add("c2", "u2", "Other", "local", "coder")
	add("c3", "u1", "Cloud", "openai", "gpt")
	add("c4", "u1", "Done", "local", "phi")
	run("r1", "c1", "running")
	run("r2", "c2", "running")
	run("r3", "c3", "running")
	run("r4", "c4", "completed")

	got, err := conversation_query.NewTurnRunQueryRepo(db).FindRunningOnPlatform("local")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	want := map[string]conversation_query.RunningTurn{
		"c1": {ConversationID: "c1", Title: "CV import", UserID: "u1", Model: "phi"},
		"c2": {ConversationID: "c2", Title: "Other", UserID: "u2", Model: "coder"},
	}
	for _, g := range got {
		if g != want[g.ConversationID] {
			t.Errorf("%+v", g)
		}
	}
	none, err := conversation_query.NewTurnRunQueryRepo(db).FindRunningOnPlatform("nothing")
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("a platform with no running turn gives an empty list, never nil: %v %v", none, err)
	}
}
