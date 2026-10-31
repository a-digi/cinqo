package jobmatcher

import (
	"regexp"
	"strings"
)

var (
	reRemote = regexp.MustCompile(`\b(?:remote|home ?office|homeoffice|hybrid|mobiles? arbeiten|mobile work|work from home|telearbeit|teleworking)\b`)
	reZip    = regexp.MustCompile(`\b\d{4,5}\b`)
	locStop  = setOf("deutschland germany de usa us uk united states kingdom und and umgebung raum region metropolregion bundesland")
)

// cityWords cut a place down to the words that name it: postcode, country and filler words removed
// ("20095 Hamburg-Altona" gives hamburg, altona).
func cityWords(s string) []string {
	s = reZip.ReplaceAllString(fold(s), " ")
	var out []string
	for _, w := range tokenize(s) {
		if len(w) >= 3 && !stopWords[w] && !locStop[w] {
			out = append(out, w)
		}
	}
	return out
}

// locationFit judges how well the job's place and way of working fit the persona. With a remote preference set
// (onsite, open, remote_only) the job's work mode is read from its text and the table of step 130 applies; without one
// the old reading is kept.
func locationFit(pj preparedJob, p Persona) (value float64, evaluable bool, note string) {
	switch p.RemotePreference {
	case "onsite", "open", "remote_only":
		return preferenceLocationFit(pj, p)
	}
	return legacyLocationFit(pj, p)
}

// legacyLocationFit is the reading for a persona that has not set a remote preference, exactly as before step 130: any
// mention of remote or home office in the job counts, and the word "remote" typed into the persona's places says it
// wants remote.
func legacyLocationFit(pj preparedJob, p Persona) (value float64, evaluable bool, note string) {
	jobWords := cityWords(pj.job.Location)
	head := pj.job.Description
	if len(head) > 2000 {
		head = head[:2000]
	}
	remote := reRemote.MatchString(fold(pj.job.Location + " " + pj.job.Title + " " + head))
	if len(jobWords) == 0 && !remote {
		return 0, false, "this job has no location"
	}
	places := cityWords(p.Location + " " + p.DesiredLocations)
	personaRemote := reRemote.MatchString(fold(p.DesiredLocations + " " + p.Location))
	if len(places) == 0 && !personaRemote {
		return 0, false, "this persona has no location or desired location"
	}
	for _, w := range jobWords {
		for _, pl := range places {
			if w == pl {
				return 1, true, "the job's place is one the persona wants or lives in: " + strings.TrimSpace(pj.job.Location)
			}
		}
	}
	if remote {
		if personaRemote {
			return 1, true, "the job is remote or hybrid and the persona wants that"
		}
		return LocationRemote, true, "the job is remote or hybrid, its place is not the persona's"
	}
	return 0, true, "the job's place is not one the persona wants or lives in"
}

// rulesOut reports whether the persona's explicit remote preference rules this job out: remote only against an on-site
// job, on-site only against a remote job. Hybrid is never ruled out, and neither is a job of unknown mode.
func rulesOut(pj preparedJob, p Persona) bool {
	switch p.RemotePreference {
	case "remote_only":
		return pj.workMode == WorkOnSite
	case "onsite":
		return pj.workMode == WorkRemote
	}
	return false
}

// preferenceLocationFit is the table of step 130: the persona's preference against how the job is worked and whether
// the job's place is one the persona wants (a city word in common).
//
//	persona wants        remote   hybrid, city   hybrid, elsewhere   on-site, city   on-site, elsewhere
//	On-site only          0        1              0                   1               0
//	Open to remote/hybrid 1        1              LocationOpenHybridElsewhere   1     0
//	Remote only           1        LocationRemoteOnlyHybrid (both hybrid cases)   0   0
func preferenceLocationFit(pj preparedJob, p Persona) (value float64, evaluable bool, note string) {
	mode := pj.workMode
	if mode == WorkUnknown {
		return 0, false, "this job does not say how it is worked: no mention of remote work and too little text to tell"
	}
	pref := p.RemotePreference
	// A remote-only persona does not care where a job is, only whether it is remote.
	if pref == "remote_only" {
		switch mode {
		case WorkRemote:
			return 1, true, "the persona wants remote work only and the job is remote"
		case WorkHybrid:
			return LocationRemoteOnlyHybrid, true, "the persona wants remote work only and the job is hybrid"
		}
		return 0, true, "the persona wants remote work only and the job is on-site"
	}
	// A fully remote job: place does not matter for a persona that is open to it, and does not help one that is not.
	if mode == WorkRemote {
		if pref == "open" {
			return 1, true, "the job is remote and the persona is open to remote work"
		}
		return 0, true, "the job is remote and the persona wants on-site work only"
	}
	// Hybrid and on-site jobs: the place decides.
	jobWords := cityWords(pj.job.Location)
	places := cityWords(p.Location + " " + p.DesiredLocations)
	switch {
	case len(jobWords) == 0:
		return 0, false, "this job is " + mode.String() + " but names no place"
	case len(places) == 0:
		return 0, false, "this persona has no location or desired location"
	}
	fits := false
	for _, w := range jobWords {
		for _, pl := range places {
			fits = fits || w == pl
		}
	}
	switch {
	case fits:
		return 1, true, "the job is " + mode.String() + " in a place the persona wants or lives in: " + strings.TrimSpace(pj.job.Location)
	case mode == WorkHybrid && pref == "open":
		return LocationOpenHybridElsewhere, true, "the job is hybrid in another place and the persona is open to hybrid work"
	}
	return 0, true, "the job is " + mode.String() + " in a place the persona does not want"
}
