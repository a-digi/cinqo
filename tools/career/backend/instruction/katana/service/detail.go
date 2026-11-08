package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"career-tool-backend/crawl"
	"career-tool-backend/db"
	"career-tool-backend/portal"
	"career-tool-backend/portal/detailpick"
)

// The job-detail operations (plan 192, step 8): the AI picks, for a crawled job's page, where the title, company, location, date and
// description are, tests the picks on real jobs and saves them. Everything runs through the screen's own routes (callScreen), so the picking
// the AI does is the picking a person does; the AI only reads a compact view of the candidates and of the test.

const (
	maxFieldCandidates = 3
	maxValueRunes      = 70
	maxMessageRunes    = 160
	maxStructuredHints = 3
)

type linkJobBody struct {
	PortalLinkID string `json:"portalLinkId"`
	JobID        string `json:"jobId,omitempty"`
}

type candidateView struct {
	Selector  string   `json:"selector"`
	Attribute string   `json:"attribute,omitempty"`
	Value     string   `json:"value"`
	Chars     int      `json:"chars,omitempty"`
	Unique    bool     `json:"unique,omitempty"`
	Why       []string `json:"why,omitempty"`
}

type structuredView struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type excludeView struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind,omitempty"`
	Risky    bool   `json:"risky,omitempty"`
}

// jobPageView is the compact picture of one job page: top candidates per field with the reasons, the page's own notices and the choices the top
// candidates make (the default of the test and the save).
type jobPageView struct {
	Page      string                     `json:"page"`
	Title     string                     `json:"title,omitempty"`
	SavedJobs int                        `json:"savedJobs"`
	Notices   []map[string]string        `json:"notices,omitempty"`
	Fields    map[string][]candidateView `json:"fields"`
	// Description help for the top description box.
	Excludes   []excludeView    `json:"descriptionExcludes,omitempty"`
	CutAfter   []string         `json:"descriptionCutAfter,omitempty"`
	Structured []structuredView `json:"structured,omitempty"`
	Suggested  picks            `json:"suggested"`
	Next       string           `json:"next"`
}

// picks is what the AI chooses: an element for each field the page has (leave a field out when the page does not state it) and the
// description box, optionally with what to leave out of it.
type elementPick struct {
	Selector  string `json:"selector"`
	Attribute string `json:"attribute,omitempty"`
}

type picks struct {
	Title               *elementPick `json:"title,omitempty"`
	Company             *elementPick `json:"company,omitempty"`
	Location            *elementPick `json:"location,omitempty"`
	PostedAt            *elementPick `json:"postedAt,omitempty"`
	DescriptionSelector string       `json:"descriptionSelector,omitempty"`
	DescriptionExclude  []string     `json:"descriptionExclude,omitempty"`
	DescriptionCutAfter []string     `json:"descriptionCutAfter,omitempty"`
}

func (p picks) empty() bool {
	return p.Title == nil && p.Company == nil && p.Location == nil && p.PostedAt == nil && p.DescriptionSelector == ""
}

// choices turns the picks into the picker's choices (the same structure the screen sends).
func (p picks) choices() detailpick.Choices {
	var c detailpick.Choices
	add := func(label detailpick.Field, e *elementPick) {
		if e != nil && strings.TrimSpace(e.Selector) != "" {
			c.Fields = append(c.Fields, detailpick.FieldChoice{Label: label, Selector: e.Selector, Attribute: e.Attribute})
		}
	}
	add(detailpick.FieldTitle, p.Title)
	add(detailpick.FieldCompany, p.Company)
	add(detailpick.FieldLocation, p.Location)
	add(detailpick.FieldPostedAt, p.PostedAt)
	if len(p.DescriptionExclude) > 0 || len(p.DescriptionCutAfter) > 0 {
		c.Description = &portal.DescriptionRule{Parts: []portal.DescriptionPart{{Include: p.DescriptionSelector, Exclude: p.DescriptionExclude, CutAfter: p.DescriptionCutAfter}}}
	} else {
		c.DescriptionSelector = p.DescriptionSelector
	}
	return c
}

type screenCandidates struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Samples []struct {
		JobID string `json:"jobId"`
	} `json:"samples"`
	JobCount int `json:"jobCount"`
	Ranked   map[string][]struct {
		Selector  string   `json:"selector"`
		Attribute string   `json:"attribute"`
		Value     string   `json:"value"`
		Chars     int      `json:"chars"`
		Unique    bool     `json:"unique"`
		Why       []string `json:"why"`
	} `json:"ranked"`
	DescriptionHelp map[string]struct {
		Excludes []detailpick.ExcludeSuggestion `json:"excludes"`
		CutAfter []string                       `json:"cutAfter"`
	} `json:"descriptionHelp"`
	StructuredHints []detailpick.StructuredSuggestion `json:"structuredHints"`
	Notices         []detailpick.Notice               `json:"notices"`
}

func attributeOf(a string) string {
	if a == "" || a == "text" {
		return ""
	}
	return a
}

// JobPageCandidates loads one crawled job's page and answers the compact view.
func JobPageCandidates(ctx context.Context, b linkJobBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	status, raw := callScreen(crawl.JobDetailCandidatesHandler, s.token, map[string]string{"portalLinkId": s.linkID, "jobId": strings.TrimSpace(b.JobID)})
	if status != http.StatusOK {
		answerRequestIfBlocked(s.linkID, status, raw)
		return nil, screenProblem(status, raw)
	}
	var in screenCandidates
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, refuse(http.StatusBadGateway, "unreadable", "the page analysis could not be read: %s", err.Error())
	}
	v := jobPageView{Page: shortURL(in.URL), Title: clip(in.Title, 80), SavedJobs: in.JobCount, Fields: map[string][]candidateView{}}
	for _, n := range in.Notices {
		v.Notices = append(v.Notices, map[string]string{"code": n.Code, "message": clip(n.Message, 200)})
	}
	var topDescription string
	var suggested picks
	for _, f := range detailpick.Fields {
		ranked := in.Ranked[string(f)]
		for i, c := range ranked {
			if i >= maxFieldCandidates {
				break
			}
			why := c.Why
			if len(why) > 2 {
				why = why[:2]
			}
			v.Fields[string(f)] = append(v.Fields[string(f)], candidateView{
				Selector: c.Selector, Attribute: attributeOf(c.Attribute), Value: clip(c.Value, maxValueRunes), Chars: c.Chars, Unique: c.Unique, Why: why,
			})
		}
		if len(ranked) == 0 {
			continue
		}
		top := &elementPick{Selector: ranked[0].Selector, Attribute: attributeOf(ranked[0].Attribute)}
		switch f {
		case detailpick.FieldTitle:
			suggested.Title = top
		case detailpick.FieldCompany:
			suggested.Company = top
		case detailpick.FieldLocation:
			suggested.Location = top
		case detailpick.FieldPostedAt:
			suggested.PostedAt = top
		case detailpick.FieldDescription:
			topDescription = ranked[0].Selector
			suggested.DescriptionSelector = topDescription
		}
	}
	if help, ok := in.DescriptionHelp[topDescription]; ok {
		for i, e := range help.Excludes {
			if i >= maxFieldCandidates {
				break
			}
			v.Excludes = append(v.Excludes, excludeView{Selector: e.Selector, Kind: e.Kind, Risky: e.Risky})
		}
		for i, c := range help.CutAfter {
			if i >= maxFieldCandidates {
				break
			}
			v.CutAfter = append(v.CutAfter, clip(c, 80))
		}
	}
	for i, h := range in.StructuredHints {
		if i >= maxStructuredHints {
			break
		}
		v.Structured = append(v.Structured, structuredView{Field: string(h.Field), Value: clip(h.Value, maxValueRunes), Source: h.Source, Path: h.Path})
	}
	v.Suggested = suggested
	v.Next = "katana_test_job_detail_picks (omit the picks to test `suggested`), then katana_save_job_detail_picks"
	getScratch(s.linkID).detailSuggested = &suggested
	return v, nil
}

type picksBody struct {
	PortalLinkID string `json:"portalLinkId"`
	picks
}

type testJobView struct {
	Title            string            `json:"title"`
	Error            string            `json:"error,omitempty"`
	Values           map[string]string `json:"values,omitempty"`
	DescriptionChars int               `json:"descriptionChars"`
	DescriptionStart string            `json:"descriptionStart,omitempty"`
}

type flagView struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

type pickTestView struct {
	Jobs    []testJobView `json:"jobs"`
	Flags   []flagView    `json:"flags,omitempty"`
	Verdict string        `json:"verdict"`
}

// which picks the choices to work on: the ones given, else the suggestion of the last job page.
func (sc *scratch) whichPicks(given picks) (picks, *problem) {
	switch {
	case !given.empty():
		return given, nil
	case sc.detailSuggested != nil:
		return *sc.detailSuggested, nil
	}
	return picks{}, refuse(http.StatusBadRequest, "no_picks", "there are no picks yet: call katana_get_job_page_candidates first, or pass the picks")
}

// TestJobDetailPicks runs the real extraction on up to three saved jobs (nothing is saved).
func TestJobDetailPicks(ctx context.Context, b picksBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	chosen, p := sc.whichPicks(b.picks)
	if p != nil {
		return nil, p
	}
	choices := chosen.choices()
	if _, err := detailpick.BuildYAML(choices); err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_picks", "%s", clip(strings.TrimSpace(strings.TrimPrefix(err.Error(), portal.ErrInvalidCrawlInstructions.Error()+":")), 300))
	}
	status, raw := callScreen(crawl.JobDetailTestHandler, s.token, map[string]any{"portalLinkId": s.linkID, "choices": choices})
	if status != http.StatusOK {
		return nil, screenProblem(status, raw)
	}
	var in struct {
		YAML string `json:"yaml"`
		Jobs []struct {
			Title            string            `json:"title"`
			Error            string            `json:"error"`
			Values           map[string]string `json:"values"`
			DescriptionChars int               `json:"descriptionChars"`
		} `json:"jobs"`
		Flags []detailpick.Flag `json:"flags"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, refuse(http.StatusBadGateway, "unreadable", "the test result could not be read: %s", err.Error())
	}
	out := pickTestView{Jobs: []testJobView{}}
	readable, blocking, limited := 0, 0, 0
	for _, j := range in.Jobs {
		if strings.HasPrefix(j.Error, rateLimitedErrorPrefix) {
			limited++
		}
		jv := testJobView{Title: clip(j.Title, 60), Error: clip(j.Error, 120), DescriptionChars: j.DescriptionChars}
		if j.Error == "" {
			readable++
			jv.Values = map[string]string{}
			for k, val := range j.Values {
				if k == string(detailpick.FieldDescription) {
					jv.DescriptionStart = clip(val, 100)
					continue
				}
				jv.Values[k] = clip(val, maxValueRunes)
			}
		}
		out.Jobs = append(out.Jobs, jv)
	}
	for _, f := range in.Flags {
		if f.Severity == detailpick.Blocking {
			blocking++
		}
		out.Flags = append(out.Flags, flagView{Code: f.Code, Severity: string(f.Severity), Field: string(f.Field), Message: clip(f.Message, maxMessageRunes)})
	}
	switch {
	case readable == 0 && limited == len(in.Jobs) && limited > 0:
		// plan 197: the pace of requests, not a wall for this browser: waiting is the fix, Rod would meet the same limit
		out.Verdict = "the site is limiting requests: STOP and tell the person to try again in about a minute (waiting, not another engine, is the fix); do not change the picks"
	case readable == 0:
		out.Verdict = "no job page could be read: STOP if the errors say the site blocked it; otherwise the picks do not work"
	case blocking > 0:
		out.Verdict = "blocking: fix the picks (see flags) and test again"
	default:
		out.Verdict = "ok: save it with katana_save_job_detail_picks if the values look right (warnings are advice)"
		sc.detailTested[in.YAML] = true
	}
	return out, nil
}

// rateLimitedErrorPrefix starts the error text of a job page the site refused because it limits the pace of requests (crawl.rateLimitErrorPrefix).
const rateLimitedErrorPrefix = "The site is limiting requests"

// SaveJobDetailPicks stores picks whose test passed in this session, as the picker's own instructions.
func SaveJobDetailPicks(ctx context.Context, b picksBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	chosen, p := sc.whichPicks(b.picks)
	if p != nil {
		return nil, p
	}
	choices := chosen.choices()
	text, err := detailpick.BuildYAML(choices)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_picks", "%s", clip(strings.TrimSpace(strings.TrimPrefix(err.Error(), portal.ErrInvalidCrawlInstructions.Error()+":")), 300))
	}
	if !sc.detailTested[text] {
		return nil, refuse(http.StatusConflict, "not_tested", "these picks have not passed katana_test_job_detail_picks in this session: test them first")
	}
	status, raw := callScreen(crawl.JobDetailSaveHandler, s.token, map[string]any{"portalLinkId": s.linkID, "choices": choices})
	if status != http.StatusOK {
		return nil, screenProblem(status, raw)
	}
	if err := portal.MarkInstructionSaved(s.linkID, portal.InstructionKindJobDetail); err != nil {
		logf("katana: could not record the saved job details on the attempt of link %s: %v", s.linkID, err)
	}
	// the job details are instructed: the event's request for the AI to write them is answered (a second run must not start)
	_, _ = db.JobsDB.Exec(`UPDATE portal_links SET job_detail_instructions_ai_requested = 0 WHERE id = ?`, s.linkID)
	return map[string]any{"saved": true, "fields": len(choices.Fields) + 1, "next": "done: the job list and the job details are saved. Reply with one short line and make no more tool calls."}, nil
}

func logf(format string, args ...any) { log.Printf(format, args...) }

// answerRequestIfBlocked: a site that blocked the job page (a challenge, a rate limit, a refused redirect: the Browser's verdict, a 502 with its own
// code) is not something a second AI run fixes, so the event's request for the AI to write the job details is answered and the page does not
// start another run that meets the same block (plan 192, the live Indeed run: Cloudflare answered 403 for the job page). A page that merely
// could not be read (code fetch_failed) keeps the request. The person can still press Generate on the Job details tile.
func answerRequestIfBlocked(linkID string, status int, body []byte) {
	if status != http.StatusBadGateway {
		return
	}
	var verdict struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &verdict) != nil || verdict.Code == "" || verdict.Code == "fetch_failed" {
		return
	}
	_, _ = db.JobsDB.Exec(`UPDATE portal_links SET job_detail_instructions_ai_requested = 0 WHERE id = ?`, linkID)
}
