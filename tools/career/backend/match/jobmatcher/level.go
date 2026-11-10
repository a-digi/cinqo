package jobmatcher

import "strings"

var (
	juniorMarkers = []string{"junior", "jr", "entry", "trainee", "werkstudent", "praktikum", "praktikant", "ausbildung", "azubi", "einsteiger", "berufseinsteiger", "volontaer"}
	seniorMarkers = []string{"senior", "sr", "lead", "teamlead", "principal", "head", "leitung", "leiter", "leiterin", "architekt", "architect", "staff", "expert", "experte", "chief", "director"}
)

// hasMarker: a word equals the marker, or (for markers of 5+ letters) starts or ends with it (Werkstudentin,
// Teamleitung).
func hasMarker(words, markers []string) bool {
	for _, w := range words {
		for _, m := range markers {
			if w == m || (len(m) >= 5 && (strings.HasPrefix(w, m) || strings.HasSuffix(w, m))) {
				return true
			}
		}
	}
	return false
}

// levelOfWords is 0 (junior), 2 (senior) or -1 when the words name no level or both.
func levelOfWords(words []string) int {
	j, s := hasMarker(words, juniorMarkers), hasMarker(words, seniorMarkers)
	switch {
	case j && !s:
		return 0
	case s && !j:
		return 2
	}
	return -1
}

func levelOfMonths(months int) int {
	switch {
	case months < JuniorBelowMonths:
		return 0
	case months < MidBelowMonths:
		return 1
	}
	return 2
}

// levelFit compares the level the job's title states with the persona's: from its desired titles, else its total
// months of experience. Equal 1, one step 0.5, two steps 0.
func levelFit(pj preparedJob, p Persona, totalMonths int) (value float64, evaluable bool, note string) {
	jobLevel := levelOfWords(pj.titleWords)
	if jobLevel < 0 {
		return 0, false, "the job's title states no level"
	}
	personaLevel := -1
	var allDesired []string
	for _, t := range reTitleList.Split(p.DesiredTitles, -1) {
		allDesired = append(allDesired, titleWords(t)...)
	}
	if l := levelOfWords(allDesired); l >= 0 {
		personaLevel = l
	} else if totalMonths > 0 {
		personaLevel = levelOfMonths(totalMonths)
	}
	if personaLevel < 0 {
		return 0, false, "this target role has no experience and no level in its desired titles"
	}
	names := []string{"junior", "mid-level", "senior"}
	note = "the job is " + names[jobLevel] + ", the target role " + names[personaLevel]
	switch d := jobLevel - personaLevel; {
	case d == 0:
		return 1, true, note
	case d == 1 || d == -1:
		return 0.5, true, note
	}
	return 0, true, note
}
