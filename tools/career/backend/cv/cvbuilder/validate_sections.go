package cvbuilder

import (
	"fmt"

	"career-tool-backend/cv/cvbuilder/templates"
)

// How many entries of each of the profile's sections a CV may carry (plan 251); the length of every text field is maxFieldLength, as for the other fields.
const (
	maxEducationEntries = 20
	maxCourseEntries    = 30
	maxLanguageEntries  = 20
	maxHobbyEntries     = 30
)

// plainText checks one text field of a section: not too long and no markup tag (a CV is plain text; the templates escape it anyway, this keeps markup out of the
// stored data and out of the Word and Markdown files made from it).
func plainText(section, field, value string) error {
	if len(value) > maxFieldLength {
		return fmt.Errorf("a %s field is too long (%s)", section, field)
	}
	if hasHTMLTag(value) {
		return fmt.Errorf("%s fields must be plain text — no HTML/markup tags (found one in %s)", section, field)
	}
	return nil
}

// validateSections checks the education, courses, languages and hobbies of a CV.
func validateSections(data templates.CvData) error {
	if len(data.Education) > maxEducationEntries {
		return fmt.Errorf("too many education entries (max %d)", maxEducationEntries)
	}
	for _, e := range data.Education {
		for _, f := range []struct{ name, value string }{
			{"institution", e.Institution}, {"degree", e.Degree}, {"fieldOfStudy", e.FieldOfStudy}, {"startDate", e.StartDate}, {"endDate", e.EndDate}, {"description", e.Description},
		} {
			if err := plainText("education", f.name, f.value); err != nil {
				return err
			}
		}
	}
	if len(data.Courses) > maxCourseEntries {
		return fmt.Errorf("too many course entries (max %d)", maxCourseEntries)
	}
	for _, c := range data.Courses {
		for _, f := range []struct{ name, value string }{{"title", c.Title}, {"provider", c.Provider}, {"completedDate", c.CompletedDate}} {
			if err := plainText("course", f.name, f.value); err != nil {
				return err
			}
		}
	}
	if len(data.Languages) > maxLanguageEntries {
		return fmt.Errorf("too many language entries (max %d)", maxLanguageEntries)
	}
	for _, l := range data.Languages {
		for _, f := range []struct{ name, value string }{{"language", l.Language}, {"level", l.Level}} {
			if err := plainText("language", f.name, f.value); err != nil {
				return err
			}
		}
	}
	if len(data.Hobbies) > maxHobbyEntries {
		return fmt.Errorf("too many hobby entries (max %d)", maxHobbyEntries)
	}
	for _, h := range data.Hobbies {
		if err := plainText("hobby", "name", h.Name); err != nil {
			return err
		}
	}
	return nil
}
