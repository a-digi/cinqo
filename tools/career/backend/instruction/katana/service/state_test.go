package service

import "testing"

func TestNextStep(t *testing.T) {
	saved := stateJobList{Saved: true}
	cases := []struct {
		name string
		s    State
		want string
	}{
		{"login", State{Login: true}, StepStopLogin},
		{"login beats everything", State{Login: true, Crawling: true, JobList: saved}, StepStopLogin},
		{"a crawl is running", State{Crawling: true}, StepWait},
		{"another AI run", State{AIRunning: true, JobList: saved}, StepWait},
		{"nothing yet", State{}, StepJobListRule},
		{"proposal ready", State{Session: stateSession{ProposalReady: true}}, StepJobListRule},
		{"rule saved, no jobs", State{JobList: saved}, StepCrawlJobs},
		{"jobs, no details", State{JobList: saved, Jobs: stateJobs{Count: 3}}, StepJobDetails},
		{"all done", State{JobList: saved, Jobs: stateJobs{Count: 3}, JobDetails: stateJobDetails{Saved: true}}, StepDone},
		{"details saved before jobs exist", State{JobList: saved, JobDetails: stateJobDetails{Saved: true}}, StepCrawlJobs},
	}
	for _, c := range cases {
		step, reason := nextStep(c.s)
		if step != c.want || reason == "" {
			t.Errorf("%s: %q (%q), want %q", c.name, step, reason, c.want)
		}
	}
	// the reason of an unfinished list says what this session already did
	if _, r := nextStep(State{Session: stateSession{ProposalReady: true, RuleTested: true}}); r == "" || !contains(r, "save it") {
		t.Errorf("tested rule: %q", r)
	}
	// a saved part is never sent back to be redone
	if _, r := nextStep(State{JobList: saved, Jobs: stateJobs{Count: 1}}); !contains(r, "do not redo") {
		t.Errorf("job details reason: %q", r)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
