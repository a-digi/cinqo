package cvbuilder

// JobRef is a job as the delete-CV confirmation lists it.
type JobRef struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Company string `json:"company"`
}

// JobLinks is what deleting a CV document needs to know about jobs. The jobs
// package imports cvbuilder, so cvbuilder cannot import jobs; main.go sets the
// implementation once at startup (UseJobLinks). A nil JobLinks means no job
// uses any document, as in unit tests. See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
type JobLinks interface {
	// JobsUsing returns every job whose current CV is the document (by id or by
	// its Media file id), no cap.
	JobsUsing(documentID, mediaFileID string) ([]JobRef, error)
	// UnlinkDocument removes the CV link of every job that uses the document.
	UnlinkDocument(documentID, mediaFileID string) error
}

var jobLinks JobLinks

// UseJobLinks sets the job-link implementation; nil clears it.
func UseJobLinks(l JobLinks) { jobLinks = l }
