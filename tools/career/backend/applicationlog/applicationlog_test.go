package applicationlog

import (
	"errors"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
)

const (
	profA = "prof-a"
	profB = "prof-b"
	job1  = "job-1"
	job2  = "job-2"
	rec1  = "rec-1"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	for _, p := range []string{profA, profB} {
		if _, err := db.CareerDB.Exec(`INSERT INTO profiles (id, first_name, last_name, created_at) VALUES (?, 'A', 'B', datetime('now'))`, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, j := range []string{job1, job2} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company) VALUES (?, ?, 'T', 'Acme')`, j, "https://x/"+j); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO recruiters (id, first_name, last_name) VALUES (?, 'R', 'S')`, rec1); err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	old := now
	now = func() string { tick = tick.Add(time.Second); return tick.Format("2006-01-02 15:04:05") }
	t.Cleanup(func() { now = old })
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

func in(dir, ch, at string) Input { return Input{Direction: dir, Channel: ch, OccurredAt: at} }

func mustAdd(t *testing.T, p, j string, i Input) Message {
	t.Helper()
	m, err := Add(p, j, i)
	if err != nil {
		t.Fatalf("Add(%+v): %v", i, err)
	}
	return m
}

func count(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM application_messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddStoresEveryFieldAndReadsItBack(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, Input{Direction: "sent", Channel: "platform", ChannelLabel: " LinkedIn ", OccurredAt: "2026-11-05T14:30:00Z",
		Subject: " Application ", Body: "  Hello  ", RecruiterID: rec1, CounterpartName: " Rita ", CounterpartContact: " rita@x.de ",
		CVDocumentID: " cv-9 ", ExternalURL: " https://www.linkedin.com/messaging/1 "})
	want := Message{ID: m.ID, ProfileID: profA, JobID: job1, Direction: "sent", Channel: "platform", ChannelLabel: "LinkedIn", OccurredAt: "2026-11-05T14:30:00Z", HasTime: true,
		Subject: "Application", Body: "Hello", RecruiterID: rec1, CounterpartName: "Rita", CounterpartContact: "rita@x.de", CVDocumentID: "cv-9",
		ExternalURL: "https://www.linkedin.com/messaging/1", CreatedAt: m.CreatedAt}
	if m != want || m.ID == "" || m.CreatedAt == "" || m.UpdatedAt != "" {
		t.Errorf("got  %+v\nwant %+v", m, want)
	}
	got, err := ByID(m.ID)
	if err != nil || got != m {
		t.Errorf("ByID: %+v %v", got, err)
	}
}

func TestDatesAreReadInEveryShapeAndStoredInUTC(t *testing.T) {
	setup(t)
	cases := []struct {
		typed, out string
		hasTime    bool
	}{
		{"2026-11-05", "2026-11-05", false},
		{"2026-11-05T14:30", "2026-11-05T14:30:00Z", true},
		{"2026-11-05T14:30:15", "2026-11-05T14:30:15Z", true},
		{"2026-11-05 14:30", "2026-11-05T14:30:00Z", true},
		{"2026-11-05T14:30:00+02:00", "2026-11-05T12:30:00Z", true},
		{"2026-11-05T23:30:00-02:00", "2026-11-06T01:30:00Z", true},
		{"  2026-11-05  ", "2026-11-05", false},
	}
	for _, c := range cases {
		m := mustAdd(t, profA, job1, in("note", "phone", c.typed))
		if m.OccurredAt != c.out || m.HasTime != c.hasTime {
			t.Errorf("%q: got %q hasTime %v, want %q %v", c.typed, m.OccurredAt, m.HasTime, c.out, c.hasTime)
		}
	}
}

func TestHasTimeFalseKeepsOnlyTheDay(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, Input{Direction: "note", Channel: "phone", OccurredAt: "2026-11-05T14:30:00Z", HasTime: bp(false)})
	if m.OccurredAt != "2026-11-05" || m.HasTime {
		t.Errorf("%+v", m)
	}
	m = mustAdd(t, profA, job1, Input{Direction: "note", Channel: "phone", OccurredAt: "2026-11-05T14:30:00Z", HasTime: bp(true)})
	if !m.HasTime || m.OccurredAt != "2026-11-05T14:30:00Z" {
		t.Errorf("%+v", m)
	}
}

func TestAddRefusesBadInputAndWritesNothing(t *testing.T) {
	setup(t)
	good := in("sent", "email", "2026-11-05")
	with := func(f func(*Input)) Input { i := good; f(&i); return i }
	cases := map[string]struct {
		profile, job string
		in           Input
		want         error
	}{
		"unknown profile":     {"nope", job1, good, ErrUnknownProfile},
		"unknown job":         {profA, "nope", good, ErrUnknownJob},
		"empty direction":     {profA, job1, with(func(i *Input) { i.Direction = "" }), ErrInvalidDirection},
		"bad direction":       {profA, job1, with(func(i *Input) { i.Direction = "outgoing" }), ErrInvalidDirection},
		"capital direction":   {profA, job1, with(func(i *Input) { i.Direction = "Sent" }), ErrInvalidDirection},
		"empty channel":       {profA, job1, with(func(i *Input) { i.Channel = "" }), ErrInvalidChannel},
		"bad channel":         {profA, job1, with(func(i *Input) { i.Channel = "telegram" }), ErrInvalidChannel},
		"missing date":        {profA, job1, with(func(i *Input) { i.OccurredAt = "" }), ErrInvalidDate},
		"not a date":          {profA, job1, with(func(i *Input) { i.OccurredAt = "yesterday" }), ErrInvalidDate},
		"day 32":              {profA, job1, with(func(i *Input) { i.OccurredAt = "2026-11-32" }), ErrInvalidDate},
		"year 1969":           {profA, job1, with(func(i *Input) { i.OccurredAt = "1969-12-31" }), ErrInvalidDate},
		"year 2101":           {profA, job1, with(func(i *Input) { i.OccurredAt = "2101-01-01" }), ErrInvalidDate},
		"time asked, no time": {profA, job1, with(func(i *Input) { i.HasTime = bp(true) }), ErrTimeWithoutTime},
		"unknown recruiter":   {profA, job1, with(func(i *Input) { i.RecruiterID = "nobody" }), ErrUnknownRecruiter},
		"link not http":       {profA, job1, with(func(i *Input) { i.ExternalURL = "javascript:alert(1)" }), ErrInvalidURL},
		"link without a host": {profA, job1, with(func(i *Input) { i.ExternalURL = "https://" }), ErrInvalidURL},
		"link is plain text":  {profA, job1, with(func(i *Input) { i.ExternalURL = "see my mail" }), ErrInvalidURL},
		"link ftp":            {profA, job1, with(func(i *Input) { i.ExternalURL = "ftp://x.de/a" }), ErrInvalidURL},
	}
	for name, c := range cases {
		if _, err := Add(c.profile, c.job, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
	if n := count(t); n != 0 {
		t.Errorf("a refusal wrote %d rows", n)
	}
}

func TestEveryDirectionAndChannelIsAccepted(t *testing.T) {
	setup(t)
	for _, d := range directions {
		mustAdd(t, profA, job1, in(d, "email", "2026-11-05"))
	}
	for _, c := range channels {
		mustAdd(t, profA, job1, in("sent", c, "2026-11-05"))
	}
	if n := count(t); n != len(directions)+len(channels) {
		t.Errorf("%d rows", n)
	}
}

func TestTextLimits(t *testing.T) {
	setup(t)
	base := in("sent", "email", "2026-11-05")
	ok := func(f func(*Input)) error { i := base; f(&i); _, err := Add(profA, job1, i); return err }
	at := func(n int) string { return strings.Repeat("é", n) } // counted in characters, not bytes
	for name, set := range map[string]struct {
		f   func(*Input, string)
		max int
	}{
		"channelLabel":       {func(i *Input, s string) { i.ChannelLabel = s }, MaxShortText},
		"subject":            {func(i *Input, s string) { i.Subject = s }, MaxShortText},
		"counterpartName":    {func(i *Input, s string) { i.CounterpartName = s }, MaxShortText},
		"counterpartContact": {func(i *Input, s string) { i.CounterpartContact = s }, MaxShortText},
		"body":               {func(i *Input, s string) { i.Body = s }, MaxBody},
		"cvDocumentId":       {func(i *Input, s string) { i.CVDocumentID = s }, maxCVDocument},
	} {
		if err := ok(func(i *Input) { set.f(i, at(set.max)) }); err != nil {
			t.Errorf("%s at the limit: %v", name, err)
		}
		if err := ok(func(i *Input) { set.f(i, at(set.max+1)) }); err == nil {
			t.Errorf("%s over the limit was accepted", name)
		}
	}
	long := "https://x.de/" + strings.Repeat("a", MaxURL)
	if err := ok(func(i *Input) { i.ExternalURL = long }); err == nil {
		t.Error("a link over the limit was accepted")
	}
	if err := ok(func(i *Input) { i.ExternalURL = long[:MaxURL] }); err != nil {
		t.Errorf("a link at the limit: %v", err)
	}
}

func TestListIsPerPairAndOldestFirstWithTiesInWrittenOrder(t *testing.T) {
	setup(t)
	c := mustAdd(t, profA, job1, in("sent", "email", "2026-11-07"))
	a := mustAdd(t, profA, job1, in("sent", "email", "2026-11-05T10:00"))
	b1 := mustAdd(t, profA, job1, in("received", "email", "2026-11-06"))
	b2 := mustAdd(t, profA, job1, in("note", "phone", "2026-11-06"))
	mustAdd(t, profB, job1, in("sent", "email", "2026-11-01"))
	mustAdd(t, profA, job2, in("sent", "email", "2026-11-01"))
	list, err := List(profA, job1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range list {
		got = append(got, m.ID)
	}
	if want := []string{a.ID, b1.ID, b2.ID, c.ID}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", got, want)
	}
	if empty, err := List(profB, job2); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("an empty pair is an empty list, not null: %v %v", empty, err)
	}
	if _, err := List("nope", job1); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("%v", err)
	}
	if _, err := List(profA, "nope"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("%v", err)
	}
}

func TestUpdateChangesOnlyTheFieldsGiven(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, Input{Direction: "sent", Channel: "email", OccurredAt: "2026-11-05T09:00", Subject: "S", Body: "B", RecruiterID: rec1,
		CounterpartName: "N", CounterpartContact: "C", CVDocumentID: "cv", ExternalURL: "https://x.de", ChannelLabel: "L"})
	u, err := Update(m.ID, Patch{Subject: sp("New")})
	if err != nil {
		t.Fatal(err)
	}
	want := m
	want.Subject = "New"
	want.UpdatedAt = u.UpdatedAt
	if u != want || u.UpdatedAt == "" {
		t.Errorf("got  %+v\nwant %+v", u, want)
	}
	none, err := Update(m.ID, Patch{})
	if err != nil || none.Subject != "New" || none.Body != "B" || none.OccurredAt != m.OccurredAt || !none.HasTime {
		t.Errorf("an empty patch changes nothing: %+v %v", none, err)
	}
	all, err := Update(m.ID, Patch{Direction: sp("received"), Channel: sp("sms"), ChannelLabel: sp("Signal"), OccurredAt: sp("2026-12-01"), Subject: sp("s2"), Body: sp("b2"),
		RecruiterID: sp(""), CounterpartName: sp("n2"), CounterpartContact: sp("c2"), CVDocumentID: sp(""), ExternalURL: sp("")})
	if err != nil {
		t.Fatal(err)
	}
	if all.Direction != "received" || all.Channel != "sms" || all.ChannelLabel != "Signal" || all.OccurredAt != "2026-12-01" || all.HasTime || all.Subject != "s2" ||
		all.Body != "b2" || all.RecruiterID != "" || all.CounterpartName != "n2" || all.CounterpartContact != "c2" || all.CVDocumentID != "" || all.ExternalURL != "" {
		t.Errorf("%+v", all)
	}
	if all.ProfileID != profA || all.JobID != job1 || all.ID != m.ID || all.CreatedAt != m.CreatedAt {
		t.Errorf("identity changed: %+v", all)
	}
}

func TestUpdateOfTheDateAndTheFlag(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, in("sent", "email", "2026-11-05T14:30:00Z"))
	u, err := Update(m.ID, Patch{HasTime: bp(false)})
	if err != nil || u.OccurredAt != "2026-11-05" || u.HasTime {
		t.Errorf("dropping the time keeps the day: %+v %v", u, err)
	}
	if _, err := Update(m.ID, Patch{HasTime: bp(true)}); !errors.Is(err, ErrTimeWithoutTime) {
		t.Errorf("a time cannot be asked for where there is none: %v", err)
	}
	u, err = Update(m.ID, Patch{OccurredAt: sp("2026-11-09T08:15"), HasTime: bp(true)})
	if err != nil || u.OccurredAt != "2026-11-09T08:15:00Z" || !u.HasTime {
		t.Errorf("a new date and time: %+v %v", u, err)
	}
	u, err = Update(m.ID, Patch{OccurredAt: sp("2026-11-10T08:15")})
	if err != nil || !u.HasTime || u.OccurredAt != "2026-11-10T08:15:00Z" {
		t.Errorf("%+v %v", u, err)
	}
}

func TestUpdateRefusesBadInputAndChangesNothing(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, Input{Direction: "sent", Channel: "email", OccurredAt: "2026-11-05", Subject: "keep"})
	cases := map[string]struct {
		id   string
		p    Patch
		want error
	}{
		"unknown id":        {"nope", Patch{Subject: sp("x")}, ErrUnknownMessage},
		"bad direction":     {m.ID, Patch{Subject: sp("x"), Direction: sp("out")}, ErrInvalidDirection},
		"bad channel":       {m.ID, Patch{Subject: sp("x"), Channel: sp("fax")}, ErrInvalidChannel},
		"bad date":          {m.ID, Patch{Subject: sp("x"), OccurredAt: sp("soon")}, ErrInvalidDate},
		"empty date":        {m.ID, Patch{Subject: sp("x"), OccurredAt: sp("")}, ErrInvalidDate},
		"unknown recruiter": {m.ID, Patch{Subject: sp("x"), RecruiterID: sp("nobody")}, ErrUnknownRecruiter},
		"bad link":          {m.ID, Patch{Subject: sp("x"), ExternalURL: sp("file:///etc/passwd")}, ErrInvalidURL},
		"too long":          {m.ID, Patch{Subject: sp(strings.Repeat("a", MaxShortText+1))}, nil},
	}
	for name, c := range cases {
		_, err := Update(c.id, c.p)
		if c.want == nil {
			if err == nil {
				t.Errorf("%s: accepted", name)
			}
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
	got, _ := ByID(m.ID)
	if got != m {
		t.Errorf("a refused update changed the entry: %+v", got)
	}
}

func TestDelete(t *testing.T) {
	setup(t)
	a := mustAdd(t, profA, job1, in("sent", "email", "2026-11-05"))
	b := mustAdd(t, profA, job1, in("sent", "email", "2026-11-06"))
	if err := Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ByID(a.ID); !errors.Is(err, ErrUnknownMessage) {
		t.Errorf("%v", err)
	}
	if _, err := ByID(b.ID); err != nil {
		t.Errorf("another entry was removed: %v", err)
	}
	if err := Delete(a.ID); !errors.Is(err, ErrUnknownMessage) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestADeletedJobTakesItsProtocolAndNoOtherJobs(t *testing.T) {
	setup(t)
	mustAdd(t, profA, job1, in("sent", "email", "2026-11-05"))
	mustAdd(t, profB, job1, in("sent", "email", "2026-11-05"))
	keep := mustAdd(t, profA, job2, in("sent", "email", "2026-11-05"))
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = ?`, job1); err != nil {
		t.Fatal(err)
	}
	if n := count(t); n != 1 {
		t.Errorf("%d rows left, want 1", n)
	}
	if _, err := ByID(keep.ID); err != nil {
		t.Errorf("%v", err)
	}
}

func TestADeletedRecruiterKeepsTheEntryAndTheFreeName(t *testing.T) {
	setup(t)
	m := mustAdd(t, profA, job1, Input{Direction: "received", Channel: "email", OccurredAt: "2026-11-05", RecruiterID: rec1, CounterpartName: "Rita"})
	if _, err := db.JobsDB.Exec(`DELETE FROM recruiters WHERE id = ?`, rec1); err != nil {
		t.Fatal(err)
	}
	got, err := ByID(m.ID)
	if err != nil || got.RecruiterID != "" || got.CounterpartName != "Rita" {
		t.Errorf("%+v %v", got, err)
	}
}

func TestPurgeProfileRemovesOnlyThatProfilesEntries(t *testing.T) {
	setup(t)
	mustAdd(t, profA, job1, in("sent", "email", "2026-11-05"))
	mustAdd(t, profA, job2, in("sent", "email", "2026-11-05"))
	keep := mustAdd(t, profB, job1, in("sent", "email", "2026-11-05"))
	if err := PurgeProfile(profA); err != nil {
		t.Fatal(err)
	}
	if n := count(t); n != 1 {
		t.Errorf("%d rows left, want 1", n)
	}
	if _, err := ByID(keep.ID); err != nil {
		t.Errorf("%v", err)
	}
	if err := PurgeProfile("never-existed"); err != nil {
		t.Errorf("purging an unknown profile is not an error: %v", err)
	}
}

func TestTheDatabaseItselfRefusesWhatTheCodeRefuses(t *testing.T) {
	setup(t)
	for _, q := range []string{
		`INSERT INTO application_messages (id, profile_id, job_id, direction, channel, occurred_at) VALUES ('x', 'p', 'job-1', 'bad', 'email', '2026-01-01 00:00:00')`,
		`INSERT INTO application_messages (id, profile_id, job_id, direction, channel, occurred_at) VALUES ('x', 'p', 'job-1', 'sent', 'bad', '2026-01-01 00:00:00')`,
		`INSERT INTO application_messages (id, profile_id, job_id, direction, channel, occurred_at, has_time) VALUES ('x', 'p', 'job-1', 'sent', 'email', '2026-01-01 00:00:00', 2)`,
		`INSERT INTO application_messages (id, profile_id, job_id, direction, channel, occurred_at) VALUES ('x', 'p', 'no-such-job', 'sent', 'email', '2026-01-01 00:00:00')`,
	} {
		if _, err := db.JobsDB.Exec(q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
}

func TestTheYearBoundariesAreInclusive(t *testing.T) {
	setup(t)
	for _, d := range []string{"1970-01-01", "2100-12-31"} {
		if m, err := Add(profA, job1, in("note", "phone", d)); err != nil || m.OccurredAt != d {
			t.Errorf("%s: %+v %v", d, m, err)
		}
	}
}

func TestADayWithoutATimeSortsBeforeTimedEntriesOfThatDayWhateverTimeWasTyped(t *testing.T) {
	setup(t)
	timed := mustAdd(t, profA, job1, in("sent", "email", "2026-11-05T01:00:00Z"))
	day := mustAdd(t, profA, job1, Input{Direction: "note", Channel: "phone", OccurredAt: "2026-11-05T23:59:00Z", HasTime: bp(false)})
	list, _ := List(profA, job1)
	if len(list) != 2 || list[0].ID != day.ID || list[1].ID != timed.ID {
		t.Errorf("a day without a time must come first: %+v", list)
	}
	// The same through an update: the time that was dropped is really gone.
	late := mustAdd(t, profA, job1, in("sent", "email", "2026-11-05T23:00:00Z"))
	if _, err := Update(late.ID, Patch{HasTime: bp(false)}); err != nil {
		t.Fatal(err)
	}
	list, _ = List(profA, job1)
	if list[1].ID != late.ID {
		t.Errorf("an entry whose time was dropped sorts as the day: %+v", list)
	}
}
