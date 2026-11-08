package applicationloghandlers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/applicationlog"
	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/profile"
)

func alSeed(t *testing.T) (profileID, jobID, recruiterID string) {
	t.Helper()
	handlertest.SetupHandlerDB(t)
	var err error
	if profileID, err = profile.CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	jobID, recruiterID = "job-al-1", "rec-al-1"
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company) VALUES (?, 'https://x/1', 'T', 'Acme')`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO recruiters (id, first_name, last_name) VALUES (?, 'R', 'S')`, recruiterID); err != nil {
		t.Fatal(err)
	}
	return
}

func alCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM application_messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func alMessages(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	list := []map[string]any{}
	for _, x := range out["messages"].([]any) {
		list = append(list, x.(map[string]any))
	}
	return list
}

func TestApplicationMessagesRoute(t *testing.T) {
	p, j, rec := alSeed(t)
	q := "?profileId=" + p + "&jobId=" + j

	// empty list is [], not null
	out := handlertest.PiJSON(t, handlertest.PiCall(ApplicationLogHandler, "GET", "/application-messages"+q, ""), 200)
	if l := alMessages(t, out); len(l) != 0 {
		t.Fatalf("%v", out)
	}

	out = handlertest.PiJSON(t, handlertest.PiCall(ApplicationLogHandler, "POST", "/application-messages", `{"profileId":"`+p+`","jobId":"`+j+`","direction":"sent","channel":"email","occurredAt":"2026-11-05T09:30:00Z","subject":"Application","body":"Hello","recruiterId":"`+rec+`"}`), 200)
	out = handlertest.PiJSON(t, handlertest.PiCall(ApplicationLogHandler, "POST", "/application-messages", `{"profileId":"`+p+`","jobId":"`+j+`","direction":"received","channel":"whatsapp","channelLabel":"WhatsApp","occurredAt":"2026-11-04","counterpartName":"Rita"}`), 200)
	list := alMessages(t, out)
	if len(list) != 2 || list[0]["direction"] != "received" || list[0]["occurredAt"] != "2026-11-04" || list[0]["hasTime"] != false || list[1]["subject"] != "Application" || list[1]["hasTime"] != true {
		t.Fatalf("oldest first, both shapes of date: %v", list)
	}
	id := list[1]["id"].(string)

	// a partial update answers with the pair's list and changes only what was given
	out = handlertest.PiJSON(t, handlertest.PiCall(ApplicationLogHandler, "PUT", "/application-messages", `{"id":"`+id+`","subject":"Re: Application","recruiterId":""}`), 200)
	var found map[string]any
	for _, m := range alMessages(t, out) {
		if m["id"] == id {
			found = m
		}
	}
	if found["subject"] != "Re: Application" || found["recruiterId"] != "" || found["body"] != "Hello" || found["channel"] != "email" || found["updatedAt"] == "" {
		t.Errorf("%v", found)
	}

	// delete
	if rc := handlertest.PiCall(ApplicationLogHandler, "DELETE", "/application-messages?id="+id, ""); rc.Code != 204 {
		t.Errorf("%d", rc.Code)
	}
	if n := alCount(t); n != 1 {
		t.Errorf("%d rows", n)
	}
}

func TestApplicationMessagesRouteRefusalsWriteNothing(t *testing.T) {
	p, j, _ := alSeed(t)
	good := `"profileId":"` + p + `","jobId":"` + j + `","direction":"sent","channel":"email","occurredAt":"2026-11-05"`
	posts := map[string]string{
		"not json":          `{`,
		"no profile":        `{"jobId":"` + j + `","direction":"sent","channel":"email","occurredAt":"2026-11-05"}`,
		"no job":            `{"profileId":"` + p + `","direction":"sent","channel":"email","occurredAt":"2026-11-05"}`,
		"unknown profile":   `{"profileId":"nope","jobId":"` + j + `","direction":"sent","channel":"email","occurredAt":"2026-11-05"}`,
		"unknown job":       `{"profileId":"` + p + `","jobId":"nope","direction":"sent","channel":"email","occurredAt":"2026-11-05"}`,
		"bad direction":     `{"profileId":"` + p + `","jobId":"` + j + `","direction":"out","channel":"email","occurredAt":"2026-11-05"}`,
		"bad channel":       `{"profileId":"` + p + `","jobId":"` + j + `","direction":"sent","channel":"fax","occurredAt":"2026-11-05"}`,
		"no date":           `{"profileId":"` + p + `","jobId":"` + j + `","direction":"sent","channel":"email"}`,
		"bad date":          `{"profileId":"` + p + `","jobId":"` + j + `","direction":"sent","channel":"email","occurredAt":"tomorrow"}`,
		"unknown recruiter": `{` + good + `,"recruiterId":"nobody"}`,
		"bad link":          `{` + good + `,"externalUrl":"javascript:alert(1)"}`,
		"text too long":     `{` + good + `,"subject":"` + strings.Repeat("a", 301) + `"}`,
		"time without time": `{` + good + `,"hasTime":true}`,
	}
	for name, body := range posts {
		rc := handlertest.PiCall(ApplicationLogHandler, "POST", "/application-messages", body)
		if rc.Code != 400 {
			t.Errorf("%s: %d %s", name, rc.Code, rc.Body.String())
		}
		if name == "text too long" && !strings.Contains(rc.Body.String(), "subject") {
			t.Errorf("the refusal must name the field: %s", rc.Body.String())
		}
	}
	if n := alCount(t); n != 0 {
		t.Errorf("a refusal wrote %d rows", n)
	}
	handlertest.PiJSON(t, handlertest.PiCall(ApplicationLogHandler, "POST", "/application-messages", `{`+good+`}`), 200)
	var id string
	_ = db.JobsDB.QueryRow(`SELECT id FROM application_messages`).Scan(&id)
	puts := map[string]string{
		"no id":         `{"subject":"x"}`,
		"unknown id":    `{"id":"nope","subject":"x"}`,
		"bad direction": `{"id":"` + id + `","direction":"out"}`,
		"bad date":      `{"id":"` + id + `","occurredAt":"soon"}`,
		"bad link":      `{"id":"` + id + `","externalUrl":"ftp://x"}`,
	}
	for name, body := range puts {
		if rc := handlertest.PiCall(ApplicationLogHandler, "PUT", "/application-messages", body); rc.Code != 400 {
			t.Errorf("%s: %d %s", name, rc.Code, rc.Body.String())
		}
	}
	for name, c := range map[string][3]string{
		"get without ids":       {"GET", "/application-messages", ""},
		"get without job":       {"GET", "/application-messages?profileId=" + p, ""},
		"get unknown profile":   {"GET", "/application-messages?profileId=nope&jobId=" + j, ""},
		"get unknown job":       {"GET", "/application-messages?profileId=" + p + "&jobId=nope", ""},
		"delete without id":     {"DELETE", "/application-messages", ""},
		"delete unknown id":     {"DELETE", "/application-messages?id=nope", ""},
		"a method not declared": {"PATCH", "/application-messages", ""},
	} {
		want := 400
		if name == "a method not declared" {
			want = 405
		}
		if rc := handlertest.PiCall(ApplicationLogHandler, c[0], c[1], c[2]); rc.Code != want {
			t.Errorf("%s: %d %s", name, rc.Code, rc.Body.String())
		}
	}
	var subject string
	_ = db.JobsDB.QueryRow(`SELECT subject FROM application_messages WHERE id = ?`, id).Scan(&subject)
	if n := alCount(t); n != 1 || subject != "" {
		t.Errorf("a refused request changed something: %d rows, subject %q", n, subject)
	}
}

func TestDeletingAProfilePurgesItsProtocolAndNobodyElses(t *testing.T) {
	p, j, _ := alSeed(t)
	other, err := profile.CreateProfile("Bob", "M")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applicationlog.Add(p, j, applicationlog.Input{Direction: "sent", Channel: "email", OccurredAt: "2026-11-05"}); err != nil {
		t.Fatal(err)
	}
	keep, err := applicationlog.Add(other, j, applicationlog.Input{Direction: "sent", Channel: "email", OccurredAt: "2026-11-05"})
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.DeleteProfile(p); err != nil {
		t.Fatal(err)
	}
	if n := alCount(t); n != 1 {
		t.Errorf("%d rows left, want 1", n)
	}
	if _, err := applicationlog.ByID(keep.ID); err != nil {
		t.Errorf("%v", err)
	}
	// deleting a profile that never existed purges nothing
	if err := profile.DeleteProfile("never-existed"); err != nil {
		t.Fatal(err)
	}
	if n := alCount(t); n != 1 {
		t.Errorf("%d", n)
	}
}

func TestApplicationMessageTools(t *testing.T) {
	p, j, rec := alSeed(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	applicationlog.RegisterListApplicationMessages(server)
	applicationlog.RegisterAddApplicationMessage(server)
	applicationlog.RegisterUpdateApplicationMessage(server)
	applicationlog.RegisterRemoveApplicationMessage(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) (map[string]any, string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out, text, res.IsError
	}
	added, text, isErr := call("add_application_message", map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05T09:30",
		"subject": "Application", "recruiterId": rec})
	if isErr || added["id"] == nil {
		t.Fatalf("%s", text)
	}
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"unknown profile":   {map[string]any{"profileId": "nope", "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05"}, "unknown profile"},
		"unknown job":       {map[string]any{"profileId": p, "jobId": "nope", "direction": "sent", "channel": "email", "occurredAt": "2026-11-05"}, "unknown job"},
		"bad direction":     {map[string]any{"profileId": p, "jobId": j, "direction": "out", "channel": "email", "occurredAt": "2026-11-05"}, "direction"},
		"bad channel":       {map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "fax", "occurredAt": "2026-11-05"}, "channel"},
		"bad date":          {map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "soon"}, "occurredAt"},
		"unknown recruiter": {map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05", "recruiterId": "x"}, "unknown recruiter"},
		"too long":          {map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05", "body": strings.Repeat("a", 20001)}, "body"},
		"bad link":          {map[string]any{"profileId": p, "jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05", "externalUrl": "x"}, "externalUrl"},
		"no profile id":     {map[string]any{"jobId": j, "direction": "sent", "channel": "email", "occurredAt": "2026-11-05"}, "profileId"},
	} {
		if _, text, isErr := call("add_application_message", c.args); !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s: %v %s", name, isErr, text)
		}
	}
	if n := alCount(t); n != 1 {
		t.Errorf("a refused call wrote rows: %d", n)
	}
	dayOnly, text, isErr := call("add_application_message", map[string]any{"profileId": p, "jobId": j, "direction": "note", "channel": "phone", "occurredAt": "2026-11-06T15:00:00Z", "hasTime": false})
	if isErr {
		t.Fatalf("%s", text)
	}
	if m, _ := applicationlog.ByID(dayOnly["id"].(string)); m.HasTime || m.OccurredAt != "2026-11-06" {
		t.Errorf("hasTime false must keep only the day: %+v", m)
	}
	if _, text, isErr := call("remove_application_message", map[string]any{"id": dayOnly["id"]}); isErr {
		t.Fatalf("%s", text)
	}
	list, _, isErr := call("list_application_messages", map[string]any{"profileId": p, "jobId": j})
	if isErr || len(list["messages"].([]any)) != 1 {
		t.Errorf("%v", list)
	}
	if _, text, isErr := call("list_application_messages", map[string]any{"profileId": p}); !isErr || !strings.Contains(text, "jobId") {
		t.Errorf("%v %s", isErr, text)
	}
	if _, text, isErr := call("update_application_message", map[string]any{"id": added["id"], "subject": "New", "recruiterId": ""}); isErr {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("update_application_message", map[string]any{"id": "nope", "subject": "x"}); !isErr || !strings.Contains(text, "unknown message") {
		t.Errorf("%v %s", isErr, text)
	}
	if _, text, isErr := call("update_application_message", map[string]any{"id": added["id"], "channel": "fax"}); !isErr || !strings.Contains(text, "channel") {
		t.Errorf("%v %s", isErr, text)
	}
	got, _ := applicationlog.ByID(added["id"].(string))
	if got.Subject != "New" || got.RecruiterID != "" || got.Channel != "email" {
		t.Errorf("%+v", got)
	}
	if _, text, isErr := call("remove_application_message", map[string]any{"id": added["id"]}); isErr {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("remove_application_message", map[string]any{"id": added["id"]}); !isErr || !strings.Contains(text, "unknown message") {
		t.Errorf("%v %s", isErr, text)
	}
	if n := alCount(t); n != 0 {
		t.Errorf("%d", n)
	}
}

func TestManifestDeclaresTheApplicationProtocol(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes   []map[string]string `json:"routes"`
		MCPTools []struct {
			Name          string `json:"name"`
			RequiredScope string `json:"required_scope"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	methods := map[string]bool{}
	for _, r := range m.Routes {
		if r["path_suffix"] == "application-messages" {
			methods[r["method"]] = true
			if r["required_scope"] != "tool:career:jobs" {
				t.Errorf("%s: scope %q", r["method"], r["required_scope"])
			}
		}
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		if !methods[method] {
			t.Errorf("manifest.json does not declare %s application-messages", method)
		}
	}
	tools := map[string]string{}
	for _, tool := range m.MCPTools {
		tools[tool.Name] = tool.RequiredScope
	}
	for _, name := range []string{"list_application_messages", "add_application_message", "update_application_message", "remove_application_message"} {
		if scope, ok := tools[name]; !ok || scope != "tool:career:jobs" {
			t.Errorf("%s: declared %v with scope %q", name, ok, scope)
		}
	}
}
