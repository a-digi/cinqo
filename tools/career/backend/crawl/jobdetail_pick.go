// jobdetail_pick.go (plan 186, step 5) — the routes behind "pick the data of a job page":
//
//	POST /portal-links/job-detail/candidates  load ONE sample page and return ranked suggestions (the Browser reads, Career ranks)
//	POST /portal-links/job-detail/preview     pure: the choices as the instruction text, or why they are not valid
//	POST /portal-links/job-detail/test        the REAL extraction on up to three sample jobs, with flags and the diff to what is stored
//	POST /portal-links/job-detail/save        store the choices through the existing validated function
//
// Decision D1 (plan 186, step 0): no Katana gate — the picking reads one page and verifies with the real extraction, whichever engine
// is selected. A link that needs a login is refused (its pages need the person's login window, which the picking screen does not
// open); the AI flow stays for it. The sample page is a saved job of the link or an address on the link's own site, so Career never
// asks the Browser for an arbitrary address. Nothing is stored except by /save; nothing here sends page text to an AI.
//
// The test uses no request id and no pacing key: the Browser then reads each page once, headless, and never opens a visible window
// (a Cloudflare challenge is reported, not waited for); a person verifying three jobs is three page loads, and one test per link runs at
// a time.
package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"career-tool-backend/jobs"
	"career-tool-backend/portal"
	"career-tool-backend/portal/detailpick"
	"career-tool-backend/textclean"
)

const (
	browserFetchDetailCandidatesPath = "/api/v1/tools/browser/proxy/fetch-detail-candidates"
	browserCrawlPaginatedPath        = "/api/v1/tools/browser/proxy/crawl-paginated"
	maxPickSamples                   = 3
	maxPickDescriptionRunes          = 100000
	maxPickBody                      = 1 << 20
)

// ---- the Browser calls (variables so tests can script them) ----

var fetchDetailPage = fetchDetailPageViaCore

func fetchDetailPageViaCore(ctx context.Context, coreURL, accessToken, pageURL, portalID string) (detailpick.Page, error) {
	body, err := json.Marshal(map[string]string{"url": pageURL, "rateLimitKey": portalID})
	if err != nil {
		return detailpick.Page{}, err
	}
	respBody, status, err := postWithBearer(ctx, coreURL+browserFetchDetailCandidatesPath, body, accessToken)
	if err != nil {
		return detailpick.Page{}, err
	}
	switch status {
	case http.StatusOK:
		var out detailpick.Page
		if err := json.Unmarshal(respBody, &out); err != nil {
			return detailpick.Page{}, fmt.Errorf("the Browser tool's page analysis could not be read: %w", err)
		}
		return out, nil
	case http.StatusUnauthorized:
		return detailpick.Page{}, errors.New("session expired — please sign in again")
	case http.StatusBadGateway:
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
			Advice  string `json:"advice"`
		}
		if json.Unmarshal(respBody, &e) == nil && e.Code != "" {
			msg := e.Message
			if e.Reason != "" {
				msg += " (" + e.Reason + ")"
			}
			return detailpick.Page{}, &linkFetchError{Code: e.Code, Message: msg, Advice: e.Advice}
		}
	case http.StatusBadRequest:
		return detailpick.Page{}, &linkFetchError{Code: "bad_request", Message: strings.TrimSpace(string(respBody))}
	}
	return detailpick.Page{}, fmt.Errorf("the Browser tool answered %d: %s", status, truncateText(strings.TrimSpace(string(respBody)), 200))
}

// pickExtraction is what the real extraction returned for one sample job.
type pickExtraction struct {
	Values      map[string]string
	Description *portal.DescriptionReport
	Truncated   []string
}

var runPickExtraction = runPickExtractionViaCore

func runPickExtractionViaCore(ctx context.Context, coreURL, accessToken string, req portal.CrawlRequest, jobURL string) (pickExtraction, error) {
	pagRequest := struct {
		portal.CrawlRequest
		URL string `json:"url"`
	}{CrawlRequest: req, URL: jobURL}
	body, err := json.Marshal(pagRequest)
	if err != nil {
		return pickExtraction{}, err
	}
	respBody, err := callBrowserProxy(ctx, coreURL, browserCrawlPaginatedPath, body, accessToken)
	if err != nil {
		return pickExtraction{}, err
	}
	var result jobDetailExtractResult
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Pages) == 0 {
		return pickExtraction{}, errNoPageExtracted
	}
	page := result.Pages[0]
	return pickExtraction{Values: extractResultValues(page), Description: page.Description, Truncated: page.Truncated}, nil
}

// ---- checks shared by the routes ----

type pickContext struct {
	coreURL, token, linkURL, portalID, linkID string
}

// checkPickLink verifies the link exists and is not a login link. needBrowser also needs the session and the core address (the
// routes that call the Browser).
func checkPickLink(w http.ResponseWriter, r *http.Request, portalLinkID string, needBrowser bool) (pickContext, bool) {
	var pc pickContext
	pc.linkID = strings.TrimSpace(portalLinkID)
	if pc.linkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return pc, false
	}
	var err error
	if pc.linkURL, err = portal.GetPortalLinkURL(pc.linkID); err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return pc, false
	}
	requiresLogin, _, err := portal.GetPortalLinkLogin(pc.linkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return pc, false
	}
	if requiresLogin {
		http.Error(w, "this link needs a login: picking the data of its job pages is not available (use the AI instructions)", http.StatusBadRequest)
		return pc, false
	}
	if needBrowser {
		cookie, cerr := r.Cookie("access_token")
		if cerr != nil {
			http.Error(w, "no active session cookie found", http.StatusUnauthorized)
			return pc, false
		}
		pc.token = cookie.Value
		if pc.coreURL, err = coreAPIURL(); err != nil {
			http.Error(w, "core API address is not configured", http.StatusInternalServerError)
			return pc, false
		}
	}
	if pc.portalID, err = portal.GetPortalIDForLink(pc.linkID); err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return pc, false
	}
	return pc, true
}

// pickSample is a saved job offered as a sample page.
type pickSample struct {
	JobID            string `json:"jobId"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	DescriptionChars int    `json:"descriptionChars"`
}

// listPickSamples offers up to n saved jobs of the link, the oldest crawled first, no two with the same title.
func listPickSamples(linkID string, n int) ([]pickSample, error) {
	targets, err := jobs.ListJobsForDetailCrawl(linkID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []pickSample{}
	for _, t := range targets {
		key := strings.ToLower(strings.TrimSpace(t.Title))
		if seen[key] || strings.TrimSpace(t.SourceURL) == "" {
			continue
		}
		seen[key] = true
		rt, err := jobs.GetRecrawlTarget(t.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, pickSample{JobID: t.ID, Title: t.Title, URL: t.SourceURL, DescriptionChars: len([]rune(rt.Description))})
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// jobOfLink returns the saved job when it belongs to the link.
func jobOfLink(linkID, jobID string) (jobs.RecrawlTarget, error) {
	t, err := jobs.GetRecrawlTarget(strings.TrimSpace(jobID))
	if err != nil {
		return jobs.RecrawlTarget{}, err
	}
	if t.PortalLinkID != linkID {
		return jobs.RecrawlTarget{}, errors.New("that job does not belong to this link")
	}
	return t, nil
}

func writePickJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodePickBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPickBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ---- candidates ----

type pickCandidatesBody struct {
	PortalLinkID string `json:"portalLinkId"`
	// JobID or URL choose the sample page (a saved job of the link, or an address on the link's own site); none: the oldest saved job.
	JobID string `json:"jobId,omitempty"`
	URL   string `json:"url,omitempty"`
}

type pickDescriptionHelp struct {
	Excludes []detailpick.ExcludeSuggestion `json:"excludes"`
	CutAfter []string                       `json:"cutAfter"`
}

type pickCurrent struct {
	YAML    string              `json:"yaml"`
	Choices *detailpick.Choices `json:"choices,omitempty"`
	// Lossy lists what the stored instructions hold that the screen cannot show (saving would drop it).
	Lossy []string `json:"lossy"`
}

type pickCandidatesResponse struct {
	URL      string                         `json:"url"`
	FinalURL string                         `json:"finalUrl"`
	Title    string                         `json:"title"`
	Samples  []pickSample                   `json:"samples"`
	Ranked   map[string][]detailpick.Ranked `json:"ranked"`
	// DescriptionHelp is keyed by the selector of a description candidate.
	DescriptionHelp map[string]pickDescriptionHelp    `json:"descriptionHelp"`
	Structured      detailpick.Structured             `json:"structured"`
	StructuredHints []detailpick.StructuredSuggestion `json:"structuredHints"`
	Elements        []detailpick.Element              `json:"elements"`
	PageTextChars   int                               `json:"pageTextChars"`
	Truncated       bool                              `json:"truncated"`
	Current         pickCurrent                       `json:"current"`
}

func currentInstructions(linkID string) pickCurrent {
	cur := pickCurrent{Lossy: []string{}}
	text, err := portal.GetPortalLinkJobDetailCrawlInstructions(linkID)
	if err != nil || strings.TrimSpace(text) == "" {
		return cur
	}
	cur.YAML = text
	if c, lossy, perr := detailpick.ParseYAML(text); perr == nil {
		cur.Choices = &c
		if lossy != nil {
			cur.Lossy = lossy
		}
	}
	return cur
}

// JobDetailCandidatesHandler handles POST /portal-links/job-detail/candidates.
func JobDetailCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	var body pickCandidatesBody
	if !decodePickBody(w, r, &body) {
		return
	}
	pc, ok := checkPickLink(w, r, body.PortalLinkID, true)
	if !ok {
		return
	}
	samples, err := listPickSamples(pc.linkID, maxPickSamples)
	if err != nil {
		http.Error(w, "failed to list the sample jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var pageURL string
	switch {
	case strings.TrimSpace(body.JobID) != "" && strings.TrimSpace(body.URL) != "":
		http.Error(w, "give either jobId or url, not both", http.StatusBadRequest)
		return
	case strings.TrimSpace(body.JobID) != "":
		t, jerr := jobOfLink(pc.linkID, body.JobID)
		if jerr != nil {
			http.Error(w, jerr.Error(), http.StatusBadRequest)
			return
		}
		pageURL = t.SourceURL
	case strings.TrimSpace(body.URL) != "":
		if pageURL, err = ownSitePage(pc.linkURL, body.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case len(samples) > 0:
		pageURL = samples[0].URL
	default:
		http.Error(w, "this link has no saved jobs yet: give the address of a job page (url)", http.StatusBadRequest)
		return
	}
	if _, err := ownSitePage(pc.linkURL, pageURL); err != nil {
		// a saved job of another site (a link can hold jobs that redirect elsewhere): never load it
		http.Error(w, "that job's page is not on the link's own site", http.StatusBadRequest)
		return
	}
	page, err := fetchDetailPage(r.Context(), pc.coreURL, pc.token, pageURL, pc.portalID)
	if err != nil {
		writeFetchFailure(w, err)
		return
	}
	resp := pickCandidatesResponse{
		URL: pageURL, FinalURL: page.FinalURL, Title: page.Title, Samples: samples,
		Ranked:          map[string][]detailpick.Ranked{},
		DescriptionHelp: map[string]pickDescriptionHelp{},
		Structured:      page.Structured, StructuredHints: detailpick.StructuredSuggestions(page),
		Elements: page.Elements, PageTextChars: page.PageTextChars, Truncated: page.Truncated,
		Current: currentInstructions(pc.linkID),
	}
	if resp.Elements == nil {
		resp.Elements = []detailpick.Element{}
	}
	for f, ranked := range detailpick.RankAll(page) {
		resp.Ranked[string(f)] = ranked
	}
	for _, c := range page.Candidates.Description {
		resp.DescriptionHelp[c.Selector] = pickDescriptionHelp{Excludes: nonNilExcludes(detailpick.SuggestExcludes(c)), CutAfter: nonNilStrings(detailpick.SuggestCutAfter(c))}
	}
	writePickJSON(w, http.StatusOK, resp)
}

func nonNilExcludes(l []detailpick.ExcludeSuggestion) []detailpick.ExcludeSuggestion {
	if l == nil {
		return []detailpick.ExcludeSuggestion{}
	}
	return l
}

func nonNilStrings(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// ---- preview and save ----

type pickChoicesBody struct {
	PortalLinkID string             `json:"portalLinkId"`
	Choices      detailpick.Choices `json:"choices"`
}

type pickPreviewResponse struct {
	Valid bool   `json:"valid"`
	YAML  string `json:"yaml,omitempty"`
	Error string `json:"error,omitempty"`
}

// JobDetailPreviewHandler handles POST /portal-links/job-detail/preview: pure, it loads and stores nothing. Choices that are not valid
// are an answer (valid: false, with the reason), not an error, so the screen can say why while the person edits.
func JobDetailPreviewHandler(w http.ResponseWriter, r *http.Request) {
	var body pickChoicesBody
	if !decodePickBody(w, r, &body) {
		return
	}
	if _, ok := checkPickLink(w, r, body.PortalLinkID, false); !ok {
		return
	}
	text, err := detailpick.BuildYAML(body.Choices)
	if err != nil {
		if detailpick.IsInvalid(err) {
			writePickJSON(w, http.StatusOK, pickPreviewResponse{Valid: false, Error: pickErrorText(err)})
			return
		}
		http.Error(w, "failed to build the instructions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writePickJSON(w, http.StatusOK, pickPreviewResponse{Valid: true, YAML: text})
}

// pickErrorText is the refusal without the technical prefix of ErrInvalidCrawlInstructions.
func pickErrorText(err error) string {
	return strings.TrimSpace(strings.TrimPrefix(err.Error(), portal.ErrInvalidCrawlInstructions.Error()+":"))
}

// JobDetailSaveHandler handles POST /portal-links/job-detail/save: the choices become the link's job-detail instructions through the
// same validated function the AI's tool uses.
func JobDetailSaveHandler(w http.ResponseWriter, r *http.Request) {
	var body pickChoicesBody
	if !decodePickBody(w, r, &body) {
		return
	}
	pc, ok := checkPickLink(w, r, body.PortalLinkID, false)
	if !ok {
		return
	}
	text, err := detailpick.BuildYAML(body.Choices)
	if err != nil {
		if detailpick.IsInvalid(err) {
			http.Error(w, pickErrorText(err), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to build the instructions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(pc.linkID, text); err != nil {
		if errors.Is(err, portal.ErrInvalidCrawlInstructions) {
			http.Error(w, pickErrorText(err), http.StatusBadRequest)
			return
		}
		portal.WritePortalLinkAwareError(w, "save the instructions", err)
		return
	}
	writePickJSON(w, http.StatusOK, pickPreviewResponse{Valid: true, YAML: text})
}

// ---- test ----

type pickTestBody struct {
	PortalLinkID string             `json:"portalLinkId"`
	Choices      detailpick.Choices `json:"choices"`
	// JobIDs are the sample jobs (at most three, all of this link); none: the oldest saved jobs.
	JobIDs []string `json:"jobIds,omitempty"`
}

type pickTestJob struct {
	JobID string `json:"jobId"`
	Title string `json:"title"`
	URL   string `json:"url"`
	// Error is set when this job's page could not be read; the other jobs still are.
	Error string `json:"error,omitempty"`
	// Values are what the page said, by label; the description is the text as it would be saved (markup out, whitespace tidied).
	Values           map[string]string         `json:"values"`
	PostedAtClean    string                    `json:"postedAtClean,omitempty"`
	DescriptionChars int                       `json:"descriptionChars"`
	Truncated        []string                  `json:"truncated,omitempty"`
	Report           *portal.DescriptionReport `json:"report,omitempty"`
	StoredChars      int                       `json:"storedChars"`
	Diff             *detailpick.DiffSummary   `json:"diff,omitempty"`
}

type pickTestResponse struct {
	YAML  string            `json:"yaml"`
	Jobs  []pickTestJob     `json:"jobs"`
	Flags []detailpick.Flag `json:"flags"`
}

// pickTestLocks lets one test per link run at a time.
var pickTestLocks sync.Map

func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// JobDetailTestHandler handles POST /portal-links/job-detail/test: the REAL extraction (the Browser's crawl-paginated, the call the
// detail crawl makes) on up to three sample jobs. Nothing is saved.
func JobDetailTestHandler(w http.ResponseWriter, r *http.Request) {
	var body pickTestBody
	if !decodePickBody(w, r, &body) {
		return
	}
	pc, ok := checkPickLink(w, r, body.PortalLinkID, true)
	if !ok {
		return
	}
	text, err := detailpick.BuildYAML(body.Choices)
	if err != nil {
		if detailpick.IsInvalid(err) {
			http.Error(w, pickErrorText(err), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to build the instructions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req, err := portal.BuildJobDetailCrawlRequestFromYAML(text)
	if err != nil {
		http.Error(w, pickErrorText(err), http.StatusBadRequest)
		return
	}
	var targets []jobs.RecrawlTarget
	switch {
	case len(body.JobIDs) > maxPickSamples:
		http.Error(w, fmt.Sprintf("at most %d sample jobs", maxPickSamples), http.StatusBadRequest)
		return
	case len(body.JobIDs) > 0:
		seen := map[string]bool{}
		for _, id := range body.JobIDs {
			if seen[id] {
				http.Error(w, "a sample job is listed twice", http.StatusBadRequest)
				return
			}
			seen[id] = true
			t, jerr := jobOfLink(pc.linkID, id)
			if jerr != nil {
				http.Error(w, jerr.Error(), http.StatusBadRequest)
				return
			}
			targets = append(targets, t)
		}
	default:
		samples, serr := listPickSamples(pc.linkID, maxPickSamples)
		if serr != nil {
			http.Error(w, "failed to list the sample jobs: "+serr.Error(), http.StatusInternalServerError)
			return
		}
		for _, s := range samples {
			t, jerr := jobs.GetRecrawlTarget(s.JobID)
			if jerr != nil {
				http.Error(w, jerr.Error(), http.StatusInternalServerError)
				return
			}
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		http.Error(w, "this link has no saved jobs to test on", http.StatusBadRequest)
		return
	}
	for _, t := range targets {
		if _, err := ownSitePage(pc.linkURL, t.SourceURL); err != nil {
			http.Error(w, fmt.Sprintf("the page of %q is not on the link's own site", t.Title), http.StatusBadRequest)
			return
		}
	}
	lock, _ := pickTestLocks.LoadOrStore(pc.linkID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	if !mu.TryLock() {
		http.Error(w, "a test of this link is already running", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	chosen := make([]detailpick.Field, 0, len(body.Choices.Fields))
	for _, f := range body.Choices.Fields {
		chosen = append(chosen, f.Label)
	}
	resp := pickTestResponse{YAML: text, Jobs: []pickTestJob{}, Flags: []detailpick.Flag{}}
	var samples []detailpick.Sample
	for _, t := range targets {
		job := pickTestJob{JobID: t.ID, Title: t.Title, URL: t.SourceURL, Values: map[string]string{}, StoredChars: len([]rune(t.Description))}
		ex, xerr := runPickExtraction(r.Context(), pc.coreURL, pc.token, req, t.SourceURL)
		if xerr != nil {
			if r.Context().Err() != nil {
				return
			}
			job.Error = truncateText(xerr.Error(), 300)
			resp.Jobs = append(resp.Jobs, job)
			continue
		}
		for k, v := range ex.Values {
			job.Values[k] = v
		}
		desc := textclean.Description(ex.Values[string(detailpick.FieldDescription)])
		job.Values[string(detailpick.FieldDescription)] = capRunes(desc, maxPickDescriptionRunes)
		job.DescriptionChars = len([]rune(desc))
		if raw := job.Values[string(detailpick.FieldPostedAt)]; raw != "" {
			job.PostedAtClean = portal.CleanDetailPostedAt(raw)
		}
		job.Truncated, job.Report = ex.Truncated, ex.Description
		d := detailpick.Diff(t.Description, desc)
		job.Diff = &d
		resp.Jobs = append(resp.Jobs, job)
		samples = append(samples, detailpick.Sample{JobID: t.ID, SavedTitle: t.Title, Values: map[string]string{
			"title": job.Values["title"], "company": job.Values["company"], "location": job.Values["location"],
			"postedAt": job.Values["postedAt"], "description": desc,
		}, Description: toPickReport(ex.Description)})
	}
	if len(samples) > 0 {
		resp.Flags = detailpick.Flags(samples, chosen)
	}
	writePickJSON(w, http.StatusOK, resp)
}

func toPickReport(d *portal.DescriptionReport) *detailpick.DescriptionReport {
	if d == nil {
		return nil
	}
	out := &detailpick.DescriptionReport{Chars: d.Chars, FullLength: d.FullLength, Truncated: d.Truncated, Limit: d.Limit, Errors: d.Errors}
	for _, p := range d.Parts {
		out.Parts = append(out.Parts, detailpick.PartReport{Index: p.Index, Matches: p.Matches, Chars: p.Chars, Overlap: p.Overlap, NotFound: p.NotFound, Cut: p.Cut, Removed: p.Removed})
	}
	return out
}
