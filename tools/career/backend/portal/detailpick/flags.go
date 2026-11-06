package detailpick

import (
	"fmt"
	"strings"

	"career-tool-backend/portal"
)

// Severity of a flag: only an empty description blocks a save; a warning asks the person to look; info is for the record.
type Severity string

const (
	Blocking Severity = "blocking"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

// The caps the save applies to what a page says about a job (jobs.FillMissingJobFields): a longer value means the selector caught a block
// of text.
const (
	MaxCompanyRunes  = 200
	MaxLocationRunes = 300
	MaxDateRunes     = 40
	// ShortDescriptionChars is the length under which a description is flagged.
	ShortDescriptionChars = 300
)

// PartReport is the Browser's report of one description part.
type PartReport struct {
	Index    int  `json:"index"`
	Matches  int  `json:"matches"`
	Chars    int  `json:"chars"`
	Overlap  bool `json:"overlap"`
	NotFound bool `json:"notFound"`
	Cut      int  `json:"cut"`
	Removed  int  `json:"removed"`
}

// DescriptionReport is the part of the Browser's description report the flags look at.
type DescriptionReport struct {
	Chars      int          `json:"chars"`
	FullLength int          `json:"fullLength"`
	Truncated  bool         `json:"truncated"`
	Limit      int          `json:"limit"`
	Parts      []PartReport `json:"parts"`
	Errors     []string     `json:"errors"`
}

// Sample is what the real extraction returned for one sample job.
type Sample struct {
	JobID      string
	SavedTitle string
	// Values are the extracted texts by label (title, company, location, postedAt, description).
	Values      map[string]string
	Description *DescriptionReport
}

// Flag is something about the extraction a person should look at before saving.
type Flag struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Field    Field    `json:"field,omitempty"`
	Message  string   `json:"message"`
	JobIDs   []string `json:"jobIds,omitempty"`
}

type flagSet struct {
	flags []Flag
	index map[string]int
}

func (fs *flagSet) add(code string, sev Severity, field Field, jobID, msg string) {
	key := code + "|" + string(field)
	if i, ok := fs.index[key]; ok {
		fs.flags[i].JobIDs = append(fs.flags[i].JobIDs, jobID)
		return
	}
	fs.index[key] = len(fs.flags)
	fs.flags = append(fs.flags, Flag{Code: code, Severity: sev, Field: field, Message: msg, JobIDs: []string{jobID}})
}

// minTitleRunesForCheck and containsTheTitle are the rule of jobs.containsTheTitle (the save ignores a company or a place that contains the
// job's own title): case-insensitive containment, for a title of at least four characters. Kept in step by hand: the pure package may not
// import the jobs database.
const minTitleRunesForCheck = 4

func containsTheTitle(value, title string) bool {
	title = strings.TrimSpace(title)
	if runeLen(title) < minTitleRunesForCheck {
		return false
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(title))
}

var furnitureLine = []string{"similar jobs", "ähnliche jobs", "ähnliche stellen", "apply now", "jetzt bewerben", "share this job", "diesen job teilen"}

// Flags checks what the extraction returned for the sample jobs. chosen are the fields the person picked (a field that was not picked is
// not "empty"). The list is in a stable order; only an empty description on every sample is Blocking.
func Flags(samples []Sample, chosen []Field) []Flag {
	fs := &flagSet{index: map[string]int{}}
	want := map[Field]bool{FieldDescription: true}
	for _, f := range chosen {
		want[f] = true
	}
	for _, s := range samples {
		get := func(f Field) string { return strings.TrimSpace(s.Values[string(f)]) }
		for _, f := range []Field{FieldTitle, FieldCompany, FieldLocation, FieldPostedAt} {
			if want[f] && get(f) == "" {
				fs.add("field_empty", Warning, f, s.JobID, fmt.Sprintf("The %s is empty on some sample jobs.", f))
			}
		}
		if n := runeLen(get(FieldCompany)); n > MaxCompanyRunes {
			fs.add("too_long", Warning, FieldCompany, s.JobID, "The company is longer than a company name can be: the selector probably holds a block of text.")
		}
		if n := runeLen(get(FieldLocation)); n > MaxLocationRunes {
			fs.add("too_long", Warning, FieldLocation, s.JobID, "The location is longer than a place can be: the selector probably holds a block of text.")
		}
		// A company or a place that holds the job's own title is a block of the page (the selector caught the heading together with the
		// name, "Backend Engineer New UNICEF" on python.org's job pages). The save ignores such a value (jobs.FillMissingJobFields uses the same
		// rule), so say it here, before the person saves: otherwise the field would just stay empty without a word.
		if containsTheTitle(get(FieldCompany), s.SavedTitle) {
			fs.add("has_the_title", Warning, FieldCompany, s.JobID, "The company contains the job's own title: the selector caught the heading together with the name. A value like that is not saved as the company.")
		}
		if containsTheTitle(get(FieldLocation), s.SavedTitle) {
			fs.add("has_the_title", Warning, FieldLocation, s.JobID, "The location contains the job's own title: the selector caught the heading together with the place. A value like that is not saved as the location.")
		}
		if raw := get(FieldPostedAt); raw != "" {
			cleaned := portal.CleanDetailPostedAt(raw)
			switch {
			case cleaned == "":
				fs.add("date_not_a_date", Warning, FieldPostedAt, s.JobID, "The posted date does not look like a date after cleaning.")
			case runeLen(cleaned) > MaxDateRunes:
				fs.add("too_long", Warning, FieldPostedAt, s.JobID, "The posted date is longer than a date can be.")
			}
		}
		// A title over several lines is a block of the page (the title with a badge and the company, found on python.org's job pages): it is
		// not a title, and the title option (replaceTitle) ignores it.
		if want[FieldTitle] && strings.Contains(get(FieldTitle), "\n") {
			fs.add("title_several_lines", Warning, FieldTitle, s.JobID, "The title spans several lines: the selector probably holds the title together with a badge or the company.")
		}
		if want[FieldTitle] && get(FieldTitle) != "" && strings.TrimSpace(s.SavedTitle) != "" && overlap(get(FieldTitle), s.SavedTitle) < 0.3 {
			fs.add("title_differs", Info, FieldTitle, s.JobID, "The title on the page is very different from the saved title of the job.")
		}
		desc := get(FieldDescription)
		if desc != "" {
			if runeLen(desc) < ShortDescriptionChars {
				fs.add("description_short", Warning, FieldDescription, s.JobID, fmt.Sprintf("The description is shorter than %d characters: the box may hold only a part.", ShortDescriptionChars))
			}
			if line, ok := furnitureLineIn(desc); ok {
				fs.add("description_furniture_words", Warning, FieldDescription, s.JobID, fmt.Sprintf("The description contains page furniture (for example %q): consider excluding it or cutting after it.", line))
			}
			if repeatedLine(desc) {
				fs.add("description_repeated_line", Warning, FieldDescription, s.JobID, "A line of the description repeats three times or more: a box may be taken twice.")
			}
		}
		if d := s.Description; d != nil {
			if d.Truncated {
				fs.add("description_cut_at_limit", Warning, FieldDescription, s.JobID, fmt.Sprintf("The description was cut at its limit (%d of %d characters).", d.Chars, d.FullLength))
			}
			overlapSeen, missing := false, false
			for _, p := range d.Parts {
				overlapSeen = overlapSeen || p.Overlap
				missing = missing || p.NotFound
			}
			if overlapSeen {
				fs.add("description_parts_overlap", Warning, FieldDescription, s.JobID, "Two parts of the description rule overlap.")
			}
			if missing {
				fs.add("description_part_not_found", Warning, FieldDescription, s.JobID, "A part of the description rule found no box on this job's page.")
			}
			if len(d.Errors) > 0 {
				fs.add("description_selector_errors", Warning, FieldDescription, s.JobID, "A selector of the description rule is not valid.")
			}
		}
	}
	// One value on every sample (two or more) for a field that should differ between jobs.
	if len(samples) >= 2 {
		for _, f := range []Field{FieldTitle, FieldDescription, FieldCompany, FieldLocation} {
			if !want[f] {
				continue
			}
			first := strings.TrimSpace(samples[0].Values[string(f)])
			if first == "" {
				continue
			}
			same := true
			for _, s := range samples[1:] {
				if strings.TrimSpace(s.Values[string(f)]) != first {
					same = false
				}
			}
			if !same {
				continue
			}
			sev := Warning
			msg := fmt.Sprintf("The %s is the same on every sample job: the selector probably holds the site's name or a menu.", f)
			if f == FieldCompany || f == FieldLocation {
				sev, msg = Info, fmt.Sprintf("The %s is the same on every sample job (possible if they are one employer or place).", f)
			}
			for _, s := range samples {
				fs.add("identical_on_all", sev, f, s.JobID, msg)
			}
		}
	}
	// An empty description blocks the save only when it is empty on every sample.
	empty := 0
	for _, s := range samples {
		if strings.TrimSpace(s.Values[string(FieldDescription)]) == "" {
			empty++
		}
	}
	if len(samples) > 0 && empty == len(samples) {
		out := Flag{Code: "description_empty", Severity: Blocking, Field: FieldDescription, Message: "The description is empty on every sample job: nothing would be saved."}
		for _, s := range samples {
			out.JobIDs = append(out.JobIDs, s.JobID)
		}
		return append([]Flag{out}, fs.flags...)
	}
	if empty > 0 {
		out := Flag{Code: "description_empty", Severity: Warning, Field: FieldDescription, Message: "The description is empty on some sample jobs."}
		for _, s := range samples {
			if strings.TrimSpace(s.Values[string(FieldDescription)]) == "" {
				out.JobIDs = append(out.JobIDs, s.JobID)
			}
		}
		return append([]Flag{out}, fs.flags...)
	}
	return fs.flags
}

func furnitureLineIn(desc string) (string, bool) {
	for _, line := range strings.Split(desc, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if l == "" {
			continue
		}
		for _, f := range furnitureLine {
			if l == f || strings.HasPrefix(l, f+" ") || strings.HasPrefix(l, f+":") {
				return strings.TrimSpace(line), true
			}
		}
		if len(l) < 200 && (strings.Contains(l, "cookies") || strings.Contains(l, "consent")) {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}

func repeatedLine(desc string) bool {
	counts := map[string]int{}
	for _, line := range strings.Split(desc, "\n") {
		l := strings.TrimSpace(line)
		if runeLen(l) < 10 {
			continue
		}
		counts[l]++
		if counts[l] >= 3 {
			return true
		}
	}
	return false
}
