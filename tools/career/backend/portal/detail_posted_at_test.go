package portal

import "testing"

func TestCleanDetailPostedAt(t *testing.T) {
	for in, want := range map[string]string{
		// a machine date (the datetime attribute of a <time> element, the live python.org value)
		"2026-10-28T08:55:19.214015+00:00": "2026-10-28",
		"2026-10-28T08:55:19Z":             "2026-10-28",
		"2026-10-28":                       "2026-10-28",
		"  2026-10-28 ":                    "2026-10-28",
		"2026-10-28 14:00":                 "2026-10-28",
		// out of range: not a date
		"2026-13-01T00:00:00Z": "",
		"2026-00-10":           "",
		"2026-10-32":           "",
		"2026-10-00":           "",
		// everything else is cleaned like a listing's date
		"23.11.2026":        "23.11.2026",
		"Posted 23-11-2026": "23-11-2026",
		"11/23/2026":        "11/23/2026",
		"vor 3 Tagen":       "", // no delimiter: unusable
		// written with month names: no digits-with-delimiters form
		"28 October 2026":        "",
		"Posted 28 October 2026": "",
		"":                       "",
		"today":                  "",
		// a year-month-day lookalike inside text is not taken as a machine date
		"on 2026-10-28 at 8": "2026-10-288",
	} {
		if got := CleanDetailPostedAt(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
