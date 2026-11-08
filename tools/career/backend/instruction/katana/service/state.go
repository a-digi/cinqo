package service

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// The state of a link's crawl instructions under Katana (plan 192, D13): what exists, what is running and the ONE next step, so that an AI
// run continues where the work stands and never redoes a finished part. Built only from what the app already stores.

// engineBlockWindow is how long a recorded block makes the AI stop; after it the AI may try again (a read that goes through clears the block).
const engineBlockWindow = time.Hour

// The next steps the state names.
const (
	StepJobListRule = "job_list_rule" // no rule yet: write it (page links, propose, test, save)
	StepCrawlJobs   = "crawl_jobs"    // the rule is saved, no job exists yet: the jobs must be crawled before the job details can be picked
	StepJobDetails  = "job_details"   // jobs exist, the job page is not instructed yet
	StepDone        = "done"          // both parts are saved: nothing to do
	StepWait        = "wait"          // a crawl or another AI run is working on this link
	StepStopLogin   = "stop_login"    // a login link without a usable kept login (none, or expired): the AI can only stop
	StepStopBlocked = "stop_blocked"  // the site stopped Katana's read a moment ago (Cloudflare, a rate limit): stop, Rod is the person's choice
)

type stateJobList struct {
	Saved      bool   `json:"saved"`
	Host       string `json:"host,omitempty"`
	Patterns   int    `json:"patterns,omitempty"`
	Pagination string `json:"pagination,omitempty"`
}

type stateJobs struct {
	Count         int    `json:"count"`
	LastCrawledAt string `json:"lastCrawledAt,omitempty"`
}

type stateJobDetails struct {
	Saved  bool   `json:"saved"`
	Origin string `json:"origin,omitempty"`
}

type stateSession struct {
	PageLoaded    bool `json:"pageLoaded"`
	ProposalReady bool `json:"proposalReady"`
	RuleTested    bool `json:"ruleTested"`
}

// State is the answer of katana_get_instruction_state.
type State struct {
	// Login: the link needs a login and no usable kept login exists (none, or it expired): the AI must stop. LoginKept: it needs one and
	// one is kept and valid, so it is read like any link (plan 195, step 5). LoginExpired: the kept login no longer works.
	Login        bool `json:"login"`
	LoginKept    bool `json:"loginKept,omitempty"`
	LoginExpired bool `json:"loginExpired,omitempty"`
	// Blocked (plan 197, step 4): the code of a block recorded within the last hour (a Katana read of this link was stopped by the site).
	// After the hour the AI may try again, and a read that goes through forgets the block.
	Blocked    string          `json:"blocked,omitempty"`
	JobList    stateJobList    `json:"jobList"`
	Jobs       stateJobs       `json:"jobs"`
	JobDetails stateJobDetails `json:"jobDetails"`
	Crawling   bool            `json:"crawling,omitempty"`
	AIRunning  bool            `json:"aiRunning,omitempty"`
	Stage      string          `json:"stage,omitempty"`
	Session    stateSession    `json:"thisSession"`
	NextStep   string          `json:"nextStep"`
	Reason     string          `json:"reason"`
}

// nextStep is the pure decision (tested on its own).
func nextStep(s State) (step, reason string) {
	switch {
	case s.Login && s.LoginExpired:
		return StepStopLogin, "the login kept for this link has expired: stop and tell the person to log in again and keep the login (Crawl now or Generate opens the window)"
	case s.Login:
		return StepStopLogin, "this link needs a login and none is kept: stop and tell the person to log in once and tick Keep this login"
	case s.Blocked != "":
		return StepStopBlocked, "the site stopped Katana's read of this link a moment ago (" + s.Blocked + "): stop and tell the person that Rod can read this link (use Rod for this link on the link's card), or that they can try again later"
	case s.Crawling:
		return StepWait, "a crawl is running for this link: do not start anything, tell the person to ask again when it has finished"
	case s.AIRunning:
		return StepWait, "another AI run is already working on this link: do not start a second one"
	case !s.JobList.Saved:
		if s.Session.RuleTested {
			return StepJobListRule, "a rule was tested in this session: save it with katana_save_link_rule"
		}
		if s.Session.ProposalReady {
			return StepJobListRule, "a rule was proposed in this session: test it with katana_test_link_rule, then save it"
		}
		return StepJobListRule, "the job list has no rule yet: katana_get_page_links, katana_propose_link_rule, katana_test_link_rule, katana_save_link_rule"
	case s.Jobs.Count == 0:
		return StepCrawlJobs, "the job list rule is saved, but no job was crawled yet: the job details can only be picked on a crawled job. Do NOT redo the rule: call katana_crawl_jobs, it crawls the jobs and waits for them"
	case !s.JobDetails.Saved:
		return StepJobDetails, "the job list is done (do not redo it); the job page still has no instructions"
	}
	return StepDone, "the job list and the job details are both saved: nothing to do. Do not change them unless the person asked"
}

// LinkState reads the state of one link.
func LinkState(ctx context.Context, b linkBody) (any, *problem) {
	id := strings.TrimSpace(b.PortalLinkID)
	if id == "" {
		return nil, refuse(http.StatusBadRequest, "bad_request", "portalLinkId is required")
	}
	if err := portal.RequirePortalLinkExists(id); err != nil {
		return nil, refuse(http.StatusNotFound, "unknown_link", "this portal link does not exist")
	}
	var s State
	login, _, err := portal.GetPortalLinkLogin(id)
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	if code, recent := portal.EngineBlockRecent(id, engineBlockWindow); recent {
		s.Blocked = code
	}
	_, loginState := portal.LoginSessionOf(id)
	s.Login = login && loginState != portal.LoginSessionValid
	s.LoginKept = login && loginState == portal.LoginSessionValid
	s.LoginExpired = login && loginState == portal.LoginSessionExpired

	var rules, detail, origin string
	err = db.JobsDB.QueryRow(
		`SELECT COALESCE(link_rules, ''), COALESCE(TRIM(job_detail_crawl_instructions), ''), job_detail_instructions_origin FROM portal_links WHERE id = ?`, id,
	).Scan(&rules, &detail, &origin)
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	if rules != "" {
		s.JobList.Saved = true
		if settings, err := portal.GetLinkCrawlSettings(id); err == nil && settings.Rules != nil {
			s.JobList.Host, s.JobList.Patterns = settings.Rules.Host, len(settings.Rules.Include)
			if p := settings.Rules.Pagination; p != nil && p.Kind != "" && p.Kind != linkrule.PagesNone {
				s.JobList.Pagination = p.Kind
			}
		}
	}
	s.JobDetails = stateJobDetails{Saved: detail != "", Origin: origin}

	var count int
	var last sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*), MAX(crawled_at) FROM jobs WHERE portal_link_id = ?`, id).Scan(&count, &last); err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	s.Jobs = stateJobs{Count: count, LastCrawledAt: last.String}

	var running int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM crawl_runs WHERE portal_link_id = ? AND status = 'running'`, id).Scan(&running); err == nil {
		s.Crawling = running > 0
	}
	if _, err := portal.ReconcileStaleInstructionAttempts(); err == nil {
		var ai int
		if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ? AND status = 'running'`, id).Scan(&ai); err == nil {
			// The attempt of THIS conversation is itself 'running'; only a second one counts. The caller passes it through selfAttempts.
			s.AIRunning = ai > selfAttempts()
		}
	}
	if links, err := portal.PipelineForLinks(portal.CrawlMethodLinks); err == nil {
		for _, l := range links {
			if l.PortalLinkID == id {
				s.Stage = l.Stage
			}
		}
	}
	sc := getScratch(id)
	s.Session = stateSession{PageLoaded: len(sc.links) > 0, ProposalReady: sc.proposed != nil}
	if sc.proposed != nil {
		s.Session.RuleTested = sc.passed[ruleKey(*sc.proposed)] > 0
	}
	s.NextStep, s.Reason = nextStep(s)
	return s, nil
}
