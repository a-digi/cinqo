package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/entity"
)

func toolsDB(t *testing.T, names ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE tool_mcp_tools (tool_id TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', input_schema TEXT NOT NULL DEFAULT '{}',
		required_scope TEXT NOT NULL DEFAULT '', media_param TEXT NOT NULL DEFAULT '', promote_media_param TEXT NOT NULL DEFAULT '', can_delegate INTEGER NOT NULL DEFAULT 0,
		delegate_allowed_tools TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if _, err := db.Exec(`INSERT INTO tool_mcp_tools (tool_id, name) VALUES ('t1', ?)`, n); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func convOn(platformID, model, mode string) *conversation_entity.Conversation {
	return &conversation_entity.Conversation{PlatformID: platformID, Model: model, OrchestrationMode: mode}
}

func TestAMessagesAllowedToolsAreCheckedLikeAConversations(t *testing.T) {
	creds := func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil }
	for id, noTools := range map[string]bool{"turn-tools-yes": false, "turn-tools-none": true} {
		if err := platform.Register(platform.Entry{Info: entity.Platform{ID: id, Name: id}, Completer: refusingCompleter{}, Credentials: creds, NoTools: noTools}); err != nil {
			t.Fatal(err)
		}
	}
	db := toolsDB(t, "pdf_to_markdown", "cv_import_set")
	direct := conversation_entity.OrchestrationModeDirect
	composer := conversation_entity.OrchestrationModeComposer
	cases := []struct {
		name   string
		conv   *conversation_entity.Conversation
		tools  []string
		status int
		text   string
	}{
		{"no list: nothing to check", convOn("turn-tools-none", "m", composer), nil, 0, ""},
		{"known tools in direct mode", convOn("turn-tools-yes", "m", direct), []string{"pdf_to_markdown", "cv_import_set"}, 0, ""},
		{"an unknown tool", convOn("turn-tools-yes", "m", direct), []string{"pdf_to_markdown", "cv_import_add_persona"}, http.StatusBadRequest, `unknown tool "cv_import_add_persona"`},
		{"composer mode", convOn("turn-tools-yes", "m", composer), []string{"pdf_to_markdown"}, http.StatusBadRequest, "only supported in direct mode"},
		{"a platform that offers no tools", convOn("turn-tools-none", "m", direct), []string{"pdf_to_markdown"}, http.StatusBadRequest, "offers no tools"},
		{"a platform that has gone is the turn's to report", convOn("no-such-platform", "m", direct), []string{"pdf_to_markdown"}, 0, ""},
	}
	for _, c := range cases {
		status, text := checkTurnAllowedTools(db, c.conv, c.tools)
		if status != c.status || (c.text != "" && !strings.Contains(text, c.text)) || (c.status == 0 && text != "") {
			t.Errorf("%s: %d %q, want %d containing %q", c.name, status, text, c.status, c.text)
		}
	}
}

func TestAMessageCanNarrowAConversationsOwnToolsButNeverWidenThem(t *testing.T) {
	db := toolsDB(t, "pdf_to_markdown", "cv_import_set", "list_jobs")
	conv := convOn("no-such-platform", "m", conversation_entity.OrchestrationModeDirect)
	conv.AllowedTools = `["pdf_to_markdown","cv_import_set"]`
	for _, c := range []struct {
		name   string
		tools  []string
		status int
		text   string
	}{
		{"a part of the conversation's tools", []string{"cv_import_set"}, 0, ""},
		{"all of them", []string{"pdf_to_markdown", "cv_import_set"}, 0, ""},
		{"a tool that exists but is not the conversation's", []string{"pdf_to_markdown", "list_jobs"}, http.StatusBadRequest, `"list_jobs" is not one of this conversation's tools`},
	} {
		status, text := checkTurnAllowedTools(db, conv, c.tools)
		if status != c.status || (c.text != "" && !strings.Contains(text, c.text)) {
			t.Errorf("%s: %d %q, want %d containing %q", c.name, status, text, c.status, c.text)
		}
	}
	// a conversation with no list of its own: any existing tool may be named
	if status, text := checkTurnAllowedTools(db, convOn("no-such-platform", "m", conversation_entity.OrchestrationModeDirect), []string{"list_jobs"}); status != 0 {
		t.Errorf("no list of its own: %d %q", status, text)
	}
}

func TestACheckOfAToolThatCannotBeLookedUpIsAServerError(t *testing.T) {
	db := toolsDB(t, "pdf_to_markdown")
	if _, err := db.Exec(`DROP TABLE tool_mcp_tools`); err != nil {
		t.Fatal(err)
	}
	status, _ := checkTurnAllowedTools(db, convOn("no-such-platform", "m", conversation_entity.OrchestrationModeDirect), []string{"pdf_to_markdown"})
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
}

func TestAMessageBodyCarriesFreshAndAllowedToolsAndNeitherByDefault(t *testing.T) {
	var with, without sendMessageRequest
	if err := json.Unmarshal([]byte(`{"content":"hi","fresh":true,"allowedTools":["pdf_to_markdown","cv_import_set"]}`), &with); err != nil {
		t.Fatal(err)
	}
	if !with.Fresh || len(with.AllowedTools) != 2 || with.AllowedTools[1] != "cv_import_set" || with.Content != "hi" {
		t.Errorf("%+v", with)
	}
	if err := json.Unmarshal([]byte(`{"content":"hi"}`), &without); err != nil {
		t.Fatal(err)
	}
	if without.Fresh || without.AllowedTools != nil {
		t.Errorf("a body without the settings changes nothing: %+v", without)
	}
}

// The handler's own wiring, pinned like the turn entry points are (a full request needs the whole server around it): the list is parsed, checked, and
// handed to the turn together with fresh, and the check comes before the turn starts.
func TestTheSendHandlerChecksTheListBeforeItStartsTheTurnWithTheOptions(t *testing.T) {
	raw, err := os.ReadFile("send_message_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	parse := strings.Index(src, "conversation_entity.ParseAllowedTools(body.AllowedTools)")
	check := strings.Index(src, "checkTurnAllowedTools(mainDB, conv, allowedTools)")
	start := strings.Index(src, "conversation.StartTurnRunWith(")
	if parse < 0 || check < 0 || start < 0 || !(parse < check && check < start) {
		t.Fatalf("the list must be parsed (%d), checked (%d) and only then the turn started (%d)", parse, check, start)
	}
	if !strings.Contains(src[start:], "conversation.TurnOptions{Fresh: body.Fresh, AllowedTools: allowedTools}") {
		t.Error("the turn must be started with the message's fresh and allowedTools")
	}
}
