package jobs

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"career-tool-backend/cvbuilder"
	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// seedPersona makes a profile with one persona and returns both ids.
func seedPersona(t *testing.T) (profileID, personaID string) {
	t.Helper()
	profileID, err := profile.CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err = persona.CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	return profileID, personaID
}

func seedCvDocument(t *testing.T, personaID, title, mediaFileID string) cvbuilder.CvDocument {
	t.Helper()
	doc, err := cvbuilder.InsertCvDocument(personaID, "classic", title, templates.CvData{}, mediaFileID, "")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func seedJob(t *testing.T, title, company string) string {
	t.Helper()
	return mustAdd(t, ManualJobInput{Title: title, Company: company}).ID
}

func currentCv(t *testing.T, jobID string) job {
	t.Helper()
	j, err := GetJobByID(jobID)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func cvRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_cv_pdfs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLinkCvAttachesAnExistingDocumentToAJob(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "My CV", "media-1")
	jobID := seedJob(t, "Backend Engineer", "Acme")

	if err := LinkCv(jobID, doc.ID, CvSourceExisting); err != nil {
		t.Fatal(err)
	}
	j := currentCv(t, jobID)
	if j.CvStatus != "completed" || j.CvMediaFileID != "media-1" || j.CvSource != "existing" || j.CvDocumentID != doc.ID || j.CvConversationID != "" || j.CvError != "" {
		t.Errorf("%+v", j)
	}
	var gotProfile string
	if err := db.JobsDB.QueryRow(`SELECT profile_id FROM job_cv_pdfs WHERE job_id = ?`, jobID).Scan(&gotProfile); err != nil || gotProfile != profileID {
		t.Errorf("profile resolved from the persona: %q %v", gotProfile, err)
	}
	// the CV document itself is untouched (no job id written onto it)
	if after, err := cvbuilder.GetCvDocument(doc.ID); err != nil || after.JobID != "" || after.MediaFileID != "media-1" {
		t.Errorf("%+v %v", after, err)
	}
}

func TestLinkCvIsIdempotentAndManualIsRecordedAsManual(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	_, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "CV", "media-1")
	jobID := seedJob(t, "Dev", "")
	for i := 0; i < 3; i++ {
		if err := LinkCv(jobID, doc.ID, CvSourceManual); err != nil {
			t.Fatal(err)
		}
	}
	if cvRows(t) != 1 || currentCv(t, jobID).CvSource != "manual" {
		t.Errorf("rows %d, %+v", cvRows(t), currentCv(t, jobID))
	}
}

func TestLinkCvReplacesAnAICvAndClearsItsState(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, personaID := seedPersona(t)
	jobID := seedJob(t, "Dev", "")
	if err := StartCvGeneration(jobID, profileID, "conv-1"); err != nil {
		t.Fatal(err)
	}
	if err := SaveGeneratedCvPdf(jobID, "media-ai", "doc-ai"); err != nil {
		t.Fatal(err)
	}
	if j := currentCv(t, jobID); j.CvSource != "ai" || j.CvMediaFileID != "media-ai" || j.CvDocumentID != "doc-ai" {
		t.Fatalf("the AI path records its own source and document: %+v", j)
	}

	doc := seedCvDocument(t, personaID, "Mine", "media-mine")
	if err := LinkCv(jobID, doc.ID, CvSourceExisting); err != nil {
		t.Fatal(err)
	}
	j := currentCv(t, jobID)
	if j.CvSource != "existing" || j.CvMediaFileID != "media-mine" || j.CvDocumentID != doc.ID || j.CvConversationID != "" {
		t.Errorf("%+v", j)
	}

	// regenerating with the AI afterwards takes the slot back as 'ai'
	if err := StartCvGeneration(jobID, profileID, "conv-2"); err != nil {
		t.Fatal(err)
	}
	if j := currentCv(t, jobID); j.CvSource != "ai" || j.CvDocumentID != "" || j.CvStatus != "generating" || j.CvMediaFileID != "" {
		t.Errorf("%+v", j)
	}
}

func TestLinkCvRefusals(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "CV", "media-1")
	noPdf := seedCvDocument(t, personaID, "Not rendered", "")
	jobID := seedJob(t, "Dev", "")

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"bad source", LinkCv(jobID, doc.ID, "ai"), ErrInvalidCvLinkSource},
		{"empty source", LinkCv(jobID, doc.ID, ""), ErrInvalidCvLinkSource},
		{"unknown job", LinkCv("nope", doc.ID, CvSourceExisting), ErrUnknownJob},
		{"unknown document", LinkCv(jobID, "nope", CvSourceExisting), cvbuilder.ErrUnknownCvDocument},
		{"no pdf", LinkCv(jobID, noPdf.ID, CvSourceExisting), ErrCvDocumentHasNoPdf},
	}
	for _, c := range cases {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.err, c.want)
		}
	}
	if cvRows(t) != 0 {
		t.Error("a refused link must write nothing")
	}

	if err := StartCvGeneration(jobID, profileID, "conv-1"); err != nil {
		t.Fatal(err)
	}
	if err := LinkCv(jobID, doc.ID, CvSourceExisting); !errors.Is(err, ErrCvGenerationRunning) {
		t.Errorf("while generating: %v", err)
	}
	if j := currentCv(t, jobID); j.CvStatus != "generating" || j.CvMediaFileID != "" {
		t.Errorf("the running generation must be untouched: %+v", j)
	}
}

func TestUnlinkCvRemovesTheCurrentCvOfAnySourceAndLeavesTheDocument(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "CV", "media-1")

	manual, existing, ai := seedJob(t, "A", ""), seedJob(t, "B", ""), seedJob(t, "C", "")
	if err := LinkCv(manual, doc.ID, CvSourceManual); err != nil {
		t.Fatal(err)
	}
	if err := LinkCv(existing, doc.ID, CvSourceExisting); err != nil {
		t.Fatal(err)
	}
	if err := StartCvGeneration(ai, profileID, "conv"); err != nil {
		t.Fatal(err)
	}
	if err := SaveGeneratedCvPdf(ai, "media-ai", "doc-ai"); err != nil {
		t.Fatal(err)
	}
	var auditBefore int
	db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_cv_generations`).Scan(&auditBefore)

	for _, id := range []string{manual, existing, ai} {
		if err := UnlinkCv(id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if j := currentCv(t, id); j.CvStatus != "" || j.CvMediaFileID != "" || j.CvSource != "" || j.CvDocumentID != "" {
			t.Errorf("%s still has a CV: %+v", id, j)
		}
	}
	if cvRows(t) != 0 {
		t.Error("rows left")
	}
	var auditAfter int
	db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_cv_generations`).Scan(&auditAfter)
	if auditAfter != auditBefore || auditAfter == 0 {
		t.Errorf("the audit trail must survive: before %d, after %d", auditBefore, auditAfter)
	}
	if after, err := cvbuilder.GetCvDocument(doc.ID); err != nil || after.MediaFileID != "media-1" {
		t.Errorf("the CV document must stay: %+v %v", after, err)
	}

	// idempotent, and the same CV can be attached again afterwards
	if err := UnlinkCv(manual); err != nil {
		t.Errorf("unlinking nothing: %v", err)
	}
	if err := LinkCv(manual, doc.ID, CvSourceExisting); err != nil {
		t.Errorf("relink: %v", err)
	}
}

func TestUnlinkCvRefusals(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, _ := seedPersona(t)
	jobID := seedJob(t, "Dev", "")
	if err := UnlinkCv("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("%v", err)
	}
	if err := StartCvGeneration(jobID, profileID, "conv"); err != nil {
		t.Fatal(err)
	}
	if err := UnlinkCv(jobID); !errors.Is(err, ErrCvGenerationRunning) {
		t.Errorf("while generating: %v", err)
	}
	if cvRows(t) != 1 {
		t.Error("the running generation must stay")
	}
}

func TestJobsUsingACvAreFoundByDocumentIdOrByMediaFileIdAndAllListed(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "Shared", "media-shared")
	other := seedCvDocument(t, personaID, "Other", "media-other")

	viaDocument := seedJob(t, "B manual", "Acme")
	viaExisting := seedJob(t, "A existing", "")
	viaMediaOnly := seedJob(t, "C ai", "Globex")
	unrelated := seedJob(t, "Z unrelated", "")
	if err := LinkCv(viaDocument, doc.ID, CvSourceManual); err != nil {
		t.Fatal(err)
	}
	if err := LinkCv(viaExisting, doc.ID, CvSourceExisting); err != nil {
		t.Fatal(err)
	}
	// an AI CV recorded only by media file id (no document id), as before step 98
	if err := StartCvGeneration(viaMediaOnly, profileID, "conv"); err != nil {
		t.Fatal(err)
	}
	if err := SaveGeneratedCvPdf(viaMediaOnly, "media-shared", ""); err != nil {
		t.Fatal(err)
	}
	if err := LinkCv(unrelated, other.ID, CvSourceExisting); err != nil {
		t.Fatal(err)
	}

	got, err := JobsUsingCv(doc.ID, doc.MediaFileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Title != "A existing" || got[1].Title != "B manual" || got[1].Company != "Acme" || got[2].Title != "C ai" {
		t.Fatalf("%+v", got)
	}

	if err := UnlinkCvDocument(doc.ID, doc.MediaFileID); err != nil {
		t.Fatal(err)
	}
	if left, _ := JobsUsingCv(doc.ID, doc.MediaFileID); len(left) != 0 {
		t.Errorf("%+v", left)
	}
	if j := currentCv(t, unrelated); j.CvDocumentID != other.ID {
		t.Errorf("an unrelated job must keep its CV: %+v", j)
	}
	if none, err := JobsUsingCv("nothing", ""); err != nil || none == nil || len(none) != 0 {
		t.Errorf("an unused document gives an empty, non-nil list: %#v %v", none, err)
	}
}

func TestAnEmptyMediaIdNeverMatchesJobsWithoutOne(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	profileID, _ := seedPersona(t)
	generating := seedJob(t, "Generating", "")
	if err := StartCvGeneration(generating, profileID, "conv"); err != nil {
		t.Fatal(err)
	}
	got, err := JobsUsingCv("some-doc", "")
	if err != nil || len(got) != 0 {
		t.Errorf("a running generation has no media id and must not match an empty one: %+v %v", got, err)
	}
	if err := UnlinkCvDocument("some-doc", ""); err != nil || cvRows(t) != 1 {
		t.Errorf("%v rows=%d", err, cvRows(t))
	}
}

func TestEveryJobUsingACvIsListed(t *testing.T) {
	setupJobsDB(t)
	captureEvents(t)
	_, personaID := seedPersona(t)
	doc := seedCvDocument(t, personaID, "Shared", "media-shared")
	for i := 0; i < 25; i++ {
		if err := LinkCv(seedJob(t, fmt.Sprintf("Job %02d", i), "Co"), doc.ID, CvSourceExisting); err != nil {
			t.Fatal(err)
		}
	}
	got, err := JobsUsingCv(doc.ID, doc.MediaFileID)
	if err != nil || len(got) != 25 {
		t.Errorf("%d jobs, %v", len(got), err)
	}
}

// A mapped CV whose Media file is gone is hidden like any other CV: source and
// document id are cleared along with the status and file id.
func TestAMappedCvWithAMissingMediaFileIsHiddenEntirely(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "t")
	list := []job{{CvStatus: "completed", CvMediaFileID: "gone", CvSource: "existing", CvDocumentID: "doc"}}
	hideDanglingCvMedia(list)
	if j := list[0]; j.CvStatus != "" || j.CvMediaFileID != "" || j.CvSource != "" || j.CvDocumentID != "" {
		t.Errorf("%+v", j)
	}
}
