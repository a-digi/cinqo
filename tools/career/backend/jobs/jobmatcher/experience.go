package jobmatcher

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reYearMonth = regexp.MustCompile(`(\d{4})\s*[-/.]\s*(\d{1,2})`)
	reMonthYear = regexp.MustCompile(`(\d{1,2})\s*[/.-]\s*(\d{4})`)
	reYear      = regexp.MustCompile(`\b(\d{4})\b`)
	reYears     = regexp.MustCompile(`(\d{1,2})\s*\+?\s*(?:jahre|jahren|jahr|years|year|yrs)\b`)
	reExperWord = regexp.MustCompile(`erfahrung|experience`)
	presentWord = setOf("present now current heute aktuell jetzt laufend")
)

// monthIndex is a calendar month as year*12 + month-1.
func monthIndex(y, m int) int { return y*12 + (m - 1) }

// parseMonth reads "2021-03", "03/2021" or "2021". A year alone is January for a start and December for an end.
func parseMonth(s string, end bool) (int, bool) {
	s = strings.TrimSpace(s)
	if m := reYearMonth.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if mo >= 1 && mo <= 12 {
			return monthIndex(y, mo), true
		}
	}
	if m := reMonthYear.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		if mo >= 1 && mo <= 12 {
			return monthIndex(y, mo), true
		}
	}
	if m := reYear.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		if end {
			return monthIndex(y, 12), true
		}
		return monthIndex(y, 1), true
	}
	return 0, false
}

type span struct{ from, to int } // inclusive month indexes

// entrySpan is the months an entry covers; ok is false when its dates cannot be read.
func entrySpan(e Experience, now time.Time) (span, bool) {
	from, ok := parseMonth(e.StartDate, false)
	if !ok {
		return span{}, false
	}
	to := monthIndex(now.Year(), int(now.Month()))
	end := strings.TrimSpace(e.EndDate)
	if end != "" {
		if presentWord[fold(end)] {
			// still there
		} else if v, ok := parseMonth(end, true); ok {
			to = v
		} else {
			return span{}, false
		}
	}
	if to < from {
		return span{}, false
	}
	return span{from, to}, true
}

// unionMonths counts the months covered by the spans, overlapping ones once.
func unionMonths(spans []span) int {
	if len(spans) == 0 {
		return 0
	}
	sorted := append([]span(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].from < sorted[j].from })
	total := 0
	cur := sorted[0]
	for _, s := range sorted[1:] {
		if s.from <= cur.to+1 {
			if s.to > cur.to {
				cur.to = s.to
			}
			continue
		}
		total += cur.to - cur.from + 1
		cur = s
	}
	return total + cur.to - cur.from + 1
}

// requiredYears reads the years of experience a job asks for ("3+ Jahre Berufserfahrung", "5 years of
// experience"): the smallest plausible number near a word about experience; DefaultRequiredYears when none.
func requiredYears(texts ...string) int {
	best := 0
	for _, t := range texts {
		t = fold(t)
		for _, loc := range reYears.FindAllStringSubmatchIndex(t, -1) {
			n, _ := strconv.Atoi(t[loc[2]:loc[3]])
			if n < MinRequiredYears || n > MaxRequiredYears {
				continue
			}
			from, to := max(0, loc[0]-60), min(len(t), loc[1]+60)
			if !reExperWord.MatchString(t[from:to]) {
				continue
			}
			if best == 0 || n < best {
				best = n
			}
		}
	}
	if best == 0 {
		return DefaultRequiredYears
	}
	return best
}

// experienceFit is the share of the years the job asks for that the persona has worked in similar roles: an entry
// counts when its title shares a word with the job's title, or its text covers two of the job's requirements (or
// holds two of the skills the job's text mentions). It also returns the persona's total months, for the level.
func experienceFit(pj preparedJob, p Persona, sk skillsOutcome, now time.Time) (value float64, evaluable bool, note string, totalMonths int) {
	if len(p.Experience) == 0 {
		return 0, false, "this persona has no experience entries", 0
	}
	var all, relevant []span
	for _, e := range p.Experience {
		sp, ok := entrySpan(e, now)
		if !ok {
			continue
		}
		all = append(all, sp)
		if entryRelevant(pj, e, sk) {
			relevant = append(relevant, sp)
		}
	}
	if len(all) == 0 {
		return 0, false, "this persona's experience entries have no readable dates", 0
	}
	months := unionMonths(relevant)
	years := requiredYears(append(pj.requirementTerms(), pj.job.Description)...)
	value = float64(months) / float64(12*years)
	if value > 1 {
		value = 1
	}
	return value, true, fmt.Sprintf("%d months in similar roles; the job asks for about %d years", months, years), unionMonths(all)
}

func entryRelevant(pj preparedJob, e Experience, sk skillsOutcome) bool {
	// the entry's title must really resemble one of the roles the job names: one shared generic word ("assistant",
	// "manager") is not similar work
	words := titleWords(e.Title)
	for _, alt := range pj.titleAlts {
		if titleSim(alt, words) >= RelevantTitleSim {
			return true
		}
	}
	tokens := tokenize(e.Title + " " + e.Description)
	n := 0
	if sk.mode == ModeRequirements {
		for _, r := range sk.reqs {
			if coveredBy(r.tokens, tokens) {
				n++
			}
		}
	} else {
		for _, ph := range sk.hits {
			if containsPhrase(tokens, ph) {
				n++
			}
		}
	}
	return n >= 2
}
