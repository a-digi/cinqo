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

// locationFit: 1 when a city word of the job is one of the persona's places, LocationRemote when the job is
// remote or hybrid and the places differ (1 for a persona that wants remote), else 0.
func locationFit(pj preparedJob, p Persona) (value float64, evaluable bool, note string) {
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
