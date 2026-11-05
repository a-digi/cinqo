package jobmatcher

import "regexp"

// WorkMode is how a job is worked, read from its text (a job has no field for it): remote, hybrid, on-site, or unknown
// when the text names neither a place nor a mode. See plan/ai/tools/career/step-130-remote-preference-and-location-score.md.
type WorkMode int

const (
	WorkUnknown WorkMode = iota
	WorkOnSite
	WorkHybrid
	WorkRemote
)

func (m WorkMode) String() string {
	switch m {
	case WorkOnSite:
		return "on-site"
	case WorkHybrid:
		return "hybrid"
	case WorkRemote:
		return "remote"
	}
	return "unknown"
}

var (
	// A strong statement that the whole job is remote (folded text: umlauts are written ae oe ue).
	reRemoteStrong = regexp.MustCompile(`\b(?:100\s*%\s*remote|(?:fully|completely|entirely)\s+remote|remote[- ]first|remote[- ]only|(?:vollstaendig|komplett|voll)\s+remote|ortsunabhaengig|work from anywhere|von ueberall(?: aus)? arbeiten)\b`)
	// "(Remote)", "- Remote" at the end of a title, or a title that starts with it.
	reRemoteTitle = regexp.MustCompile(`\(\s*(?:100\s*%\s*)?remote\s*\)|[-–|/,]\s*(?:100\s*%\s*)?remote\s*$|^\s*remote\b`)
	// "remote" in another sense: remote sensing, remote control, remote access, remote desktop, remote monitoring.
	reRemoteOtherSense = regexp.MustCompile(`\bremote\s+(?:sensing|control|controls|access|desktop|monitoring)\b`)
	// Partly remote: hybrid, some days at home, remote or home office as an option.
	reHybrid = regexp.MustCompile(`\b(?:hybrid\w*|teilweise\s+(?:remote|home\s?office|mobil\w*)|home\s?office\s+moeglich|remote\s+moeglich|home\s?office[- ]option|remote[- ]option|remote[- ]friendly|remote work possible|mobile[sn]?\s+arbeit\w*|mobile work|flexible[sn]?\s+arbeit\w*|flexible work|telearbeit|teleworking|work from home|wfh|(?:\d+|ein|zwei|drei|vier)\s+tage?\s+(?:pro woche\s+)?(?:home\s?office|remote|mobil\w*))\b`)
	// Any mention of remote or home office: some remote element, however small.
	reRemoteBare = regexp.MustCompile(`\b(?:remote|home\s?office|homeoffice)\b`)

	// words that may stand next to "remote" in a place without naming a city: "Deutschland (Remote)", "Remote, EU".
	remotePlaceFiller = setOf("remote homeoffice home office deutschland germany bundesweit deutschlandweit europe europa eu weltweit worldwide anywhere ueberall dach emea und and oder or in de at ch oesterreich austria schweiz switzerland")
)

// remoteOnlyPlace reports whether a place text is only "remote": it says remote or home office and names no city.
func remoteOnlyPlace(location string) bool {
	toks := tokenizeFolded(location)
	if len(toks) == 0 {
		return false
	}
	saysRemote := false
	for _, t := range toks {
		switch t {
		case "remote", "homeoffice":
			saysRemote = true
		case "home", "office":
			// only part of "home office"
		default:
			if !remotePlaceFiller[t] {
				return false
			}
		}
	}
	return saysRemote || (hasWord(toks, "home") && hasWord(toks, "office"))
}

func hasWord(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// jobWorkMode reads how a job is worked from its title, its place and the start of its text. Remote when it says so
// strongly (or its place is just "Remote", or its title says so); hybrid when it says partly, or mentions remote or
// home office at all (a perk in an on-site job makes it hybrid, not remote); on-site when it names a place, has enough
// text (WorkModeMinText) to have said otherwise, and says neither; unknown when it has no place and no mark, or a place
// but too little text to tell.
func jobWorkMode(j Job) WorkMode {
	loc := reRemoteOtherSense.ReplaceAllString(fold(j.Location), " ")
	title := reRemoteOtherSense.ReplaceAllString(fold(j.Title), " ")
	desc := []rune(j.Description)
	if len(desc) > 2000 {
		desc = desc[:2000]
	}
	all := title + " " + loc + " " + reRemoteOtherSense.ReplaceAllString(fold(string(desc)), " ")
	switch {
	case reRemoteStrong.MatchString(all), remoteOnlyPlace(loc), reRemoteTitle.MatchString(title):
		return WorkRemote
	case reHybrid.MatchString(all), reRemoteBare.MatchString(all):
		return WorkHybrid
	case len(cityWords(j.Location)) > 0 && len([]rune(j.Description)) >= WorkModeMinText:
		return WorkOnSite
	}
	return WorkUnknown
}
