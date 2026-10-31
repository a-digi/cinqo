package profile

import "strings"

// How complete a profile is, out of 100. Worked out on read from what the profile
// already has (never stored, so it cannot go stale); the rule is pure so it can be
// tested without a database. See
// plan/ai/tools/career/step-101-profile-completeness-and-platform-toggle.md.

// minSkills is how many skills on one persona make the Skills item done.
const minSkills = 3

// Band names, as the JSON carries them.
const (
	bandJustStarted  = "just-started"
	bandGettingThere = "getting-there"
	bandStrong       = "strong"
	bandComplete     = "complete"
)

// completenessInput is what the rule needs to know about one profile. Skills and
// experience are counts on the best single persona, not summed over personas.
type completenessInput struct {
	FirstName             string
	LastName              string
	LinkCount             int
	HasImage              bool
	PersonaCount          int
	HasHeadlineAndSummary bool
	BestSkillCount        int
	BestExperienceCount   int
}

type completenessItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Weight int    `json:"weight"`
	Done   bool   `json:"done"`
}

type completeness struct {
	Score int                `json:"score"`
	Band  string             `json:"band"`
	Items []completenessItem `json:"items"`
}

// completenessOf lists every item with whether it is done, adds up the weights of the
// done ones (the weights add up to 100) and names the band.
func completenessOf(in completenessInput) completeness {
	items := []completenessItem{
		{"name", "Name", 10, strings.TrimSpace(in.FirstName) != "" || strings.TrimSpace(in.LastName) != ""},
		{"links", "Platform links", 25, in.LinkCount >= 1},
		{"image", "Profile image", 10, in.HasImage},
		{"persona", "Persona", 15, in.PersonaCount >= 1},
		{"summary", "Headline and summary", 15, in.HasHeadlineAndSummary},
		{"skills", "Skills", 15, in.BestSkillCount >= minSkills},
		{"experience", "Experience", 10, in.BestExperienceCount >= 1},
	}
	score := 0
	for _, it := range items {
		if it.Done {
			score += it.Weight
		}
	}
	return completeness{Score: score, Band: bandOf(score), Items: items}
}

func bandOf(score int) string {
	switch {
	case score >= 100:
		return bandComplete
	case score >= 70:
		return bandStrong
	case score >= 40:
		return bandGettingThere
	default:
		return bandJustStarted
	}
}
