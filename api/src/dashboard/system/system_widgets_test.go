package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/dashboard"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/updatecheck"
)

// runThroughEngine registers p and fetches its data the way the dashboard does,
// so each widget is proven against its own kind's validation, not just its own tests.
func runThroughEngine(t *testing.T, p dashboard.Provider, caller dashboard.Caller) dashboard.Data {
	t.Helper()
	r := dashboard.NewRegistry()
	if err := r.Register(p); err != nil {
		t.Fatalf("definition rejected: %v", err)
	}
	d, ok := dashboard.NewEngine(r).Data(context.Background(), caller, p.Definition().ID())
	if !ok {
		t.Fatal("widget not visible to the caller")
	}
	return d
}

var admin = dashboard.Caller{UserID: "root", Scopes: []string{dashboard.AdminScope}}

func TestEveryDefinitionIsValidAndScoped(t *testing.T) {
	for _, p := range []dashboard.Provider{
		NewToolsOverview(nil), NewUpdatesAvailable(nil, nil, ""), NewRecentConversations(nil), NewMediaUsage(nil),
	} {
		def := p.Definition()
		if err := def.Validate(); err != nil {
			t.Errorf("%s: %v", def.ID(), err)
		}
		if def.Provider != "cinqo" || def.RequiredScope == "" {
			t.Errorf("%s: system widgets are provider cinqo and always scoped: %+v", def.ID(), def)
		}
	}
}

// --- updates ---------------------------------------------------------------

type noRepos struct{ err error }

func (n noRepos) List() ([]*repo_entity.Repository, error) { return nil, n.err }

func TestUpdatesStat(t *testing.T) {
	none := updatesStat(nil)
	if !none.Value.IsNumber || none.Value.Number != 0 || none.Caption != "All tools are up to date" {
		t.Errorf("none: %+v", none)
	}
	ups := func(names ...string) []updatecheck.Update {
		out := make([]updatecheck.Update, len(names))
		for i, n := range names {
			out[i] = updatecheck.Update{Name: n}
		}
		return out
	}
	if got := updatesStat(ups("Career", "Browser")); got.Value.Number != 2 || got.Caption != "Career, Browser" {
		t.Errorf("two: %+v", got)
	}
	if got := updatesStat(ups("A", "B", "C", "D", "E")); got.Value.Number != 5 || got.Caption != "A, B, C +2 more" {
		t.Errorf("five: %+v", got)
	}
	long := strings.Repeat("x", 100)
	if got := updatesStat(ups(long, long, long)); len([]rune(got.Caption)) > 200 {
		t.Errorf("caption not bounded: %d", len([]rune(got.Caption)))
	}
}

func TestUpdatesAvailableEndToEnd(t *testing.T) {
	db := coreDB(t)
	d := runThroughEngine(t, NewUpdatesAvailable(db, noRepos{}, "0.0.100"), admin)
	if d.State != dashboard.StateOK {
		t.Fatalf("state = %q", d.State)
	}
	var got dashboard.StatData
	if err := json.Unmarshal(d.Data, &got); err != nil || got.Value.Number != 0 || got.Link != "/admin/tools" {
		t.Errorf("stat = %+v, %v", got, err)
	}

	d = runThroughEngine(t, NewUpdatesAvailable(db, noRepos{err: errors.New("db down")}, ""), admin)
	if d.State != dashboard.StateError {
		t.Errorf("a failing repository list must be an error state, got %q", d.State)
	}
}

// --- media -----------------------------------------------------------------

func TestMediaChart(t *testing.T) {
	const mb = 1024 * 1024
	usage := []media_query.ToolUsage{
		{ToolSlug: "career", Files: 3, Bytes: 5*mb + mb/2},
		{ToolSlug: "cinqo", Files: 1, Bytes: mb},
	}
	got := mediaChart(usage)
	if got.Type != "bar" || len(got.Series) != 1 || got.Series[0].Name != "Size (MB)" {
		t.Fatalf("chart = %+v", got)
	}
	if p := got.Series[0].Points; len(p) != 2 || p[0].X != "career" || p[0].Y != 5.5 || p[1].X != "cinqo" || p[1].Y != 1 {
		t.Errorf("points = %+v", p)
	}

	many := make([]media_query.ToolUsage, 9)
	for i := range many {
		many[i] = media_query.ToolUsage{ToolSlug: string(rune('a' + i)), Bytes: int64(9-i) * mb}
	}
	p := mediaChart(many).Series[0].Points
	if len(p) != maxMediaBars+1 || p[len(p)-1].X != "Other" || p[len(p)-1].Y != 6 { // 3+2+1 MB
		t.Errorf("overflow not folded into Other: %+v", p)
	}
	if empty := mediaChart(nil).Series[0].Points; empty == nil || len(empty) != 0 {
		t.Errorf("no media must give an empty, non-nil series: %#v", empty)
	}
}

func TestMediaUsageEndToEnd(t *testing.T) {
	db := coreDB(t)
	w := NewMediaUsage(db)

	if d := runThroughEngine(t, w, admin); d.State != dashboard.StateEmpty {
		t.Errorf("no media: state = %q, want empty", d.State)
	}

	for i, row := range []struct {
		slug string
		size int64
	}{{"career", 3 * 1024 * 1024}, {"career", 1024 * 1024}, {"cinqo", 512 * 1024}} {
		if _, err := db.Exec(
			`INSERT INTO media_files (id, tool_slug, original_filename, extension, stored_path, content_type, size_bytes, uploaded_by_user_id, created_at)
			 VALUES (?, ?, 'f', '.x', '/p', 'application/octet-stream', ?, 'u', '2026-10-30T00:00:00Z')`,
			string(rune('a'+i)), row.slug, row.size,
		); err != nil {
			t.Fatal(err)
		}
	}
	d := runThroughEngine(t, w, admin)
	if d.State != dashboard.StateOK {
		t.Fatalf("state = %q", d.State)
	}
	var got dashboard.ChartData
	if err := json.Unmarshal(d.Data, &got); err != nil {
		t.Fatal(err)
	}
	p := got.Series[0].Points
	if len(p) != 2 || p[0].X != "career" || p[0].Y != 4 || p[1].X != "cinqo" || p[1].Y != 0.5 {
		t.Errorf("points = %+v", p)
	}
}

// --- conversations ---------------------------------------------------------

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	for stored, want := range map[string]string{
		"2026-10-30 11:59:40":  "just now",
		"2026-10-30 11:55:00":  "5m ago",
		"2026-10-30T09:00:00Z": "3h ago",
		"2026-10-28 12:00:00":  "2d ago",
		"2026-09-01 12:00:00":  "1 Sep 2026",
		"garbage":              "",
		"":                     "",
	} {
		if got := relativeTime(now, stored); got != want {
			t.Errorf("relativeTime(%q) = %q, want %q", stored, got, want)
		}
	}
}

func TestRecentList(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	var cs []*conversation_entity.Conversation
	for i := 0; i < 8; i++ {
		cs = append(cs, &conversation_entity.Conversation{Title: "Chat", Model: "m", StartedAt: "2026-10-30 11:00:00"})
	}
	cs[0].Title = ""
	cs[1].Title = strings.Repeat("t", 300)
	got := recentList(cs, now)
	if len(got.Items) != recentConversationLimit {
		t.Fatalf("items = %d, want %d", len(got.Items), recentConversationLimit)
	}
	if got.Items[0].Title != "Untitled conversation" || got.Items[0].Meta != "1h ago" || got.Items[0].Link != "/conversations" || got.Items[0].Subtitle != "m" {
		t.Errorf("first = %+v", got.Items[0])
	}
	if len([]rune(got.Items[1].Title)) != 120 {
		t.Errorf("long title not truncated: %d", len([]rune(got.Items[1].Title)))
	}
	if empty := recentList(nil, now); empty.Items == nil || len(empty.Items) != 0 {
		t.Errorf("no conversations must give an empty, non-nil list: %#v", empty.Items)
	}
}

func TestRecentConversationsOnlyReadsTheCallersOwn(t *testing.T) {
	db := migratedDB(t, "migrations-conversation")
	insert := func(id, user, title string, hidden int) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO conversations (id, user_id, title, started_at, file_path, hidden) VALUES (?, ?, ?, '2026-10-30 10:00:00', '/f', ?)`,
			id, user, title, hidden,
		); err != nil {
			t.Fatal(err)
		}
	}
	insert("1", "alice", "Alice chat", 0)
	insert("2", "bob", "Bob chat", 0)
	insert("3", "alice", "Alice hidden", 1)

	w := NewRecentConversations(db)
	w.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	alice := dashboard.Caller{UserID: "alice", Scopes: []string{"cinqo:conversation:use"}}

	d := runThroughEngine(t, w, alice)
	if d.State != dashboard.StateOK {
		t.Fatalf("state = %q", d.State)
	}
	var got dashboard.ListData
	if err := json.Unmarshal(d.Data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Title != "Alice chat" || got.Items[0].Meta != "2h ago" {
		t.Errorf("items = %+v (must be alice's own, visible ones only)", got.Items)
	}

	// Without the conversation scope the widget does not exist for the caller.
	r := dashboard.NewRegistry()
	_ = r.Register(w)
	if _, ok := dashboard.NewEngine(r).Data(context.Background(), dashboard.Caller{UserID: "alice"}, "cinqo.recent_conversations"); ok {
		t.Error("visible without cinqo:conversation:use")
	}

	if d := runThroughEngine(t, NewRecentConversations(db), dashboard.Caller{UserID: "carol", Scopes: []string{"cinqo:conversation:use"}}); d.State != dashboard.StateEmpty {
		t.Errorf("a user with no conversations: state = %q, want empty", d.State)
	}
}
