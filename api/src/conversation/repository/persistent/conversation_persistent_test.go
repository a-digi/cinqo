package persistent_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// openConversationDB applies every real conversation migration, in
// file-name order, to a fresh in-memory database — the same SQL the
// running app applies.
func openConversationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	dir := filepath.Join("..", "..", "..", "..", "config", "db", "migrations-conversation")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s (err %v)", dir, err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

func newConversation(id, mode string) *conversation_entity.Conversation {
	return &conversation_entity.Conversation{
		ID: id, UserID: "u1", Title: "t", FilePath: "/tmp/" + id, PlatformID: "p", Model: "m",
		OrchestrationMode: mode,
	}
}

func TestOrchestrationModeRoundTrip(t *testing.T) {
	db := openConversationDB(t)
	repo := conversation_persistent.NewConversationPersistentRepo(db)
	q := conversation_query.NewConversationQueryRepo(db)

	for id, c := range map[string]struct{ in, want string }{
		"c-direct":   {conversation_entity.OrchestrationModeDirect, conversation_entity.OrchestrationModeDirect},
		"c-composer": {conversation_entity.OrchestrationModeComposer, conversation_entity.OrchestrationModeComposer},
		"c-empty":    {"", conversation_entity.OrchestrationModeDirect}, // unset means today's behavior
	} {
		if err := repo.Insert(newConversation(id, c.in)); err != nil {
			t.Fatalf("%s: insert: %v", id, err)
		}
		got, err := q.FindOwnedByID(id, "u1")
		if err != nil || got.OrchestrationMode != c.want {
			t.Errorf("%s: mode=%q err=%v, want %q", id, got.OrchestrationMode, err, c.want)
		}
		byID, err := q.FindByID(id)
		if err != nil || byID.OrchestrationMode != c.want {
			t.Errorf("%s: FindByID mode=%q err=%v, want %q", id, byID.OrchestrationMode, err, c.want)
		}
	}

	list, err := q.ListOwnedBy("u1")
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %d rows, err %v", len(list), err)
	}
}

func TestOrchestrationModeRejectsUnknownValueAtTheDatabase(t *testing.T) {
	db := openConversationDB(t)
	repo := conversation_persistent.NewConversationPersistentRepo(db)
	if err := repo.Insert(newConversation("bad", "swarm")); err == nil {
		t.Fatal("the CHECK constraint must reject an unknown mode")
	}
}

func TestExistingRowsDefaultToDirect(t *testing.T) {
	db := openConversationDB(t)
	// A row written without the column, as every pre-existing row was.
	if _, err := db.Exec(`INSERT INTO conversations (id, user_id, title, file_path, platform_id, model, hidden) VALUES ('old', 'u1', 't', '/tmp/old', 'p', 'm', 0)`); err != nil {
		t.Fatal(err)
	}
	got, err := conversation_query.NewConversationQueryRepo(db).FindByID("old")
	if err != nil || got.OrchestrationMode != conversation_entity.OrchestrationModeDirect {
		t.Fatalf("mode=%q err=%v, want direct", got.OrchestrationMode, err)
	}
}

func TestToolIterationLimitRoundTripAndDefault(t *testing.T) {
	db := openConversationDB(t)
	repo := conversation_persistent.NewConversationPersistentRepo(db)
	q := conversation_query.NewConversationQueryRepo(db)

	plain := newConversation("c-default", conversation_entity.OrchestrationModeDirect)
	limited := newConversation("c-limited", conversation_entity.OrchestrationModeDirect)
	ten := 10
	limited.ToolIterationLimit = &ten
	for _, c := range []*conversation_entity.Conversation{plain, limited} {
		if err := repo.Insert(c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := q.FindByID("c-default")
	if err != nil || got.ToolIterationLimit != nil {
		t.Fatalf("an unset limit must read back as nil: %+v %v", got, err)
	}
	got, err = q.FindByID("c-limited")
	if err != nil || got.ToolIterationLimit == nil || *got.ToolIterationLimit != 10 {
		t.Fatalf("limit must round-trip: %+v %v", got, err)
	}
}

func TestToolIterationLimitRejectsZeroAtTheDatabase(t *testing.T) {
	db := openConversationDB(t)
	zero := 0
	c := newConversation("c-zero", conversation_entity.OrchestrationModeDirect)
	c.ToolIterationLimit = &zero
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(c); err == nil {
		t.Error("a limit of 0 must be rejected by the CHECK constraint")
	}
}

func TestAllowedToolsRoundTripAndDefault(t *testing.T) {
	db := openConversationDB(t)
	repo := conversation_persistent.NewConversationPersistentRepo(db)
	q := conversation_query.NewConversationQueryRepo(db)

	enc, err := conversation_entity.EncodeAllowedTools([]string{"list_personas", "get_persona_details"})
	if err != nil {
		t.Fatal(err)
	}
	restricted := newConversation("c-restricted", conversation_entity.OrchestrationModeDirect)
	restricted.AllowedTools = enc
	for _, c := range []*conversation_entity.Conversation{newConversation("c-open", conversation_entity.OrchestrationModeDirect), restricted} {
		if err := repo.Insert(c); err != nil {
			t.Fatal(err)
		}
	}
	// A row written before the column existed has no restriction.
	if _, err := db.Exec(`INSERT INTO conversations (id, user_id, title, file_path, platform_id, model, hidden) VALUES ('old', 'u1', 't', '/tmp/old', 'p', 'm', 0)`); err != nil {
		t.Fatal(err)
	}

	for _, read := range map[string]func(id string) (*conversation_entity.Conversation, error){
		"FindByID":      func(id string) (*conversation_entity.Conversation, error) { return q.FindByID(id) },
		"FindOwnedByID": func(id string) (*conversation_entity.Conversation, error) { return q.FindOwnedByID(id, "u1") },
	} {
		got, err := read("c-restricted")
		if err != nil || len(got.AllowedToolNames()) != 2 || got.AllowedToolNames()[0] != "list_personas" {
			t.Errorf("restricted: names=%v err=%v", got.AllowedToolNames(), err)
		}
		for _, id := range []string{"c-open", "old"} {
			got, err := read(id)
			if err != nil || got.AllowedToolNames() != nil {
				t.Errorf("%s: names=%v err=%v, want no restriction (nil)", id, got.AllowedToolNames(), err)
			}
		}
	}
	list, err := q.ListOwnedBy("u1")
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %d rows, err %v", len(list), err)
	}
	for _, c := range list {
		if (c.ID == "c-restricted") != (c.AllowedToolNames() != nil) {
			t.Errorf("list: %s has names %v", c.ID, c.AllowedToolNames())
		}
	}
}
