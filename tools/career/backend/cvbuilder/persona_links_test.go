package cvbuilder

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/comments/commentstest"
	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func profileOfPersona(t *testing.T, personaID string) string {
	t.Helper()
	var id string
	if err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func link(t *testing.T, cvID, personaID string) {
	t.Helper()
	if _, err := db.CareerDB.Exec(`INSERT INTO cv_document_personas (cv_document_id, persona_id) VALUES (?, ?)`, cvID, personaID); err != nil {
		t.Fatal(err)
	}
}

func personaNames(ps []CvPersona) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInsertCvDocument_LinksItsBuiltFromPersona(t *testing.T) {
	pid := setupCvDB(t)
	doc := insertDoc(t, pid, "CV", "media-1", "")
	if len(doc.Personas) != 1 || doc.Personas[0].ID != pid || !doc.Personas[0].BuiltFrom || doc.Personas[0].Name != "Backend" {
		t.Fatalf("personas = %+v", doc.Personas)
	}
}

func TestInsertCvDocument_NoLinkMeansNoCv(t *testing.T) {
	pid := setupCvDB(t)
	if _, err := db.CareerDB.Exec(`DROP TABLE cv_document_personas`); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertCvDocument(pid, "classic", "CV", templates.CvData{}, "media-1", ""); err == nil {
		t.Fatal("want an error when the link cannot be written")
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cv rows = %d (%v), want 0: the CV and its link are one transaction", n, err)
	}
}

func TestReads_PersonasBuiltFromFirstThenByName_AndListsFollowTheLinks(t *testing.T) {
	built := setupCvDB(t) // "Backend"
	profileID := profileOfPersona(t, built)
	zed, err := persona.CreatePersona(profileID, "Zed", "")
	if err != nil {
		t.Fatal(err)
	}
	amy, err := persona.CreatePersona(profileID, "amy", "")
	if err != nil {
		t.Fatal(err)
	}
	doc := insertDoc(t, built, "CV", "media-1", "")
	link(t, doc.ID, zed)
	link(t, doc.ID, amy)

	want := []string{"Backend", "amy", "Zed"}
	got, err := GetCvDocument(doc.ID)
	if err != nil || !same(personaNames(got.Personas), want) {
		t.Fatalf("Get: %v %v", personaNames(got.Personas), err)
	}
	if !got.Personas[0].BuiltFrom || got.Personas[1].BuiltFrom || got.Personas[2].BuiltFrom {
		t.Fatalf("builtFrom flags: %+v", got.Personas)
	}
	all, err := ListAllCvDocuments()
	if err != nil || len(all) != 1 || !same(personaNames(all[0].Personas), want) {
		t.Fatalf("All: %+v %v", all, err)
	}
	for _, pid := range []string{built, zed, amy} {
		list, err := ListCvDocuments(pid)
		if err != nil || len(list) != 1 || list[0].ID != doc.ID || !same(personaNames(list[0].Personas), want) {
			t.Fatalf("List(%s): %+v %v", pid, list, err)
		}
	}

	// A persona with no link to it does not list the CV, and a CV with no personas is a list, not null.
	other, _ := persona.CreatePersona(profileID, "Other", "")
	if list, _ := ListCvDocuments(other); len(list) != 0 {
		t.Fatalf("unlinked persona lists %d CVs", len(list))
	}
	if _, err := db.CareerDB.Exec(`DELETE FROM cv_document_personas`); err != nil {
		t.Fatal(err)
	}
	bare, _ := GetCvDocument(doc.ID)
	if bare.Personas == nil || len(bare.Personas) != 0 {
		t.Fatalf("personas = %#v, want an empty list", bare.Personas)
	}
}

func TestLinks_GoWhenEitherEndIsDeleted(t *testing.T) {
	built := setupCvDB(t)
	profileID := profileOfPersona(t, built)
	extra, _ := persona.CreatePersona(profileID, "Extra", "")
	doc := insertDoc(t, built, "CV", "media-1", "")
	link(t, doc.ID, extra)

	if _, err := db.CareerDB.Exec(`DELETE FROM personas WHERE id = ?`, extra); err != nil {
		t.Fatal(err)
	}
	got, _ := GetCvDocument(doc.ID)
	if len(got.Personas) != 1 || got.Personas[0].ID != built {
		t.Fatalf("after deleting the attached persona: %+v", got.Personas)
	}
	if _, err := DeleteCvDocument(doc.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_document_personas`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("links left after deleting the CV: %d (%v)", n, err)
	}
}

func linkedNames(t *testing.T, cvID string) []string {
	t.Helper()
	doc, err := GetCvDocument(cvID)
	if err != nil {
		t.Fatal(err)
	}
	return personaNames(doc.Personas)
}

func TestAttachCvToPersona_Rules(t *testing.T) {
	built := setupCvDB(t) // "Backend"
	profileID := profileOfPersona(t, built)
	extra, _ := persona.CreatePersona(profileID, "Extra", "")
	otherProfile, err := profile.CreateProfile("Bob", "Other")
	if err != nil {
		t.Fatal(err)
	}
	foreign, _ := persona.CreatePersona(otherProfile, "Foreign", "")
	doc := insertDoc(t, built, "CV", "media-1", "")

	if err := AttachCvToPersona(doc.ID, extra); err != nil {
		t.Fatal(err)
	}
	if err := AttachCvToPersona(doc.ID, extra); err != nil { // idempotent
		t.Fatalf("second attach: %v", err)
	}
	if err := AttachCvToPersona(doc.ID, built); err != nil { // the built-from one is already linked
		t.Fatalf("attach built-from: %v", err)
	}
	if got := linkedNames(t, doc.ID); !same(got, []string{"Backend", "Extra"}) {
		t.Fatalf("linked = %v", got)
	}

	if err := AttachCvToPersona(doc.ID, foreign); !errors.Is(err, ErrPersonaOfOtherProfile) {
		t.Fatalf("other profile: %v", err)
	}
	if err := AttachCvToPersona(doc.ID, "nope"); !errors.Is(err, persona.ErrUnknownPersona) {
		t.Fatalf("unknown persona: %v", err)
	}
	if err := AttachCvToPersona("nope", extra); !errors.Is(err, ErrUnknownCvDocument) {
		t.Fatalf("unknown cv: %v", err)
	}
	if got := linkedNames(t, doc.ID); !same(got, []string{"Backend", "Extra"}) {
		t.Fatalf("a refused attach changed the links: %v", got)
	}
}

func TestDetachCvFromPersona_Rules(t *testing.T) {
	built := setupCvDB(t)
	extra, _ := persona.CreatePersona(profileOfPersona(t, built), "Extra", "")
	doc := insertDoc(t, built, "CV", "media-1", "")
	if err := AttachCvToPersona(doc.ID, extra); err != nil {
		t.Fatal(err)
	}

	if err := DetachCvFromPersona(doc.ID, built); !errors.Is(err, ErrCannotDetachBuiltFrom) {
		t.Fatalf("built-from: %v", err)
	}
	third, _ := persona.CreatePersona(profileOfPersona(t, built), "Third", "")
	if err := AttachCvToPersona(doc.ID, third); err != nil {
		t.Fatal(err)
	}
	if err := DetachCvFromPersona(doc.ID, extra); err != nil {
		t.Fatal(err)
	}
	if got := linkedNames(t, doc.ID); !same(got, []string{"Backend", "Third"}) {
		t.Fatalf("detaching one persona changed the others: %v", got)
	}
	if err := DetachCvFromPersona(doc.ID, third); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []string{extra, "never-linked"} { // idempotent, also for a stranger
		if err := DetachCvFromPersona(doc.ID, pid); err != nil {
			t.Fatalf("detach again (%s): %v", pid, err)
		}
	}
	if err := DetachCvFromPersona("nope", extra); !errors.Is(err, ErrUnknownCvDocument) {
		t.Fatalf("unknown cv: %v", err)
	}
	if got := linkedNames(t, doc.ID); !same(got, []string{"Backend"}) {
		t.Fatalf("linked = %v", got)
	}
	// The CV and the persona survive a detach.
	if _, err := GetCvDocument(doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := requirePersonaExists(extra); err != nil {
		t.Fatal(err)
	}
}

func TestPersonaLinksHandler(t *testing.T) {
	built := setupCvDB(t)
	profileID := profileOfPersona(t, built)
	extra, _ := persona.CreatePersona(profileID, "Extra", "")
	otherProfile, _ := profile.CreateProfile("Bob", "Other")
	foreign, _ := persona.CreatePersona(otherProfile, "Foreign", "")
	doc := insertDoc(t, built, "CV", "media-1", "")

	do := func(method, target, body string) int {
		rec := httptest.NewRecorder()
		PersonaLinksHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
		return rec.Code
	}
	put := func(cv, p string) int {
		return do(http.MethodPut, "/cv-documents/personas", fmt.Sprintf(`{"cvDocumentId":%q,"personaId":%q}`, cv, p))
	}
	del := func(cv, p string) int {
		return do(http.MethodDelete, "/cv-documents/personas?cvDocumentId="+cv+"&personaId="+p, "")
	}

	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"attach", put(doc.ID, extra), 204},
		{"attach again", put(doc.ID, extra), 204},
		{"other profile", put(doc.ID, foreign), 400},
		{"unknown persona", put(doc.ID, "nope"), 404},
		{"unknown cv", put("nope", extra), 404},
		{"missing ids", put("", ""), 400},
		{"bad body", do(http.MethodPut, "/cv-documents/personas", "not json"), 400},
		{"detach built-from", del(doc.ID, built), 400},
		{"detach", del(doc.ID, extra), 204},
		{"detach again", del(doc.ID, extra), 204},
		{"detach unknown cv", del("nope", extra), 404},
		{"detach missing ids", do(http.MethodDelete, "/cv-documents/personas", ""), 400},
		{"wrong method", do(http.MethodGet, "/cv-documents/personas", ""), 405},
	} {
		if c.got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestDeleteCvDocument_PurgesItsComments_AndSurvivesACoreThatIsDown(t *testing.T) {
	pid := setupCvDB(t)
	purged := commentstest.Start(t)
	doc := insertDoc(t, pid, "CV", "media-1", "")
	if _, err := DeleteCvDocument(doc.ID); err != nil {
		t.Fatal(err)
	}
	if got := purged(); len(got) != 1 || got[0] != "cv:"+doc.ID {
		t.Fatalf("purged %v, want [cv:%s]", got, doc.ID)
	}

	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	doc2 := insertDoc(t, pid, "CV2", "media-2", "")
	if _, err := DeleteCvDocument(doc2.ID); err != nil {
		t.Fatalf("a failing purge failed the delete: %v", err)
	}
	if _, err := GetCvDocument(doc2.ID); !errors.Is(err, ErrUnknownCvDocument) {
		t.Fatalf("CV still there: %v", err)
	}
}
