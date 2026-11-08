// Package jobmatcher is Career's manual (no AI) Job Match engine: a pure, deterministic scorer that rates one
// job against the personas of a profile and answers the same 0-100 score the AI match records, with a breakdown
// of how it got there. It never touches a database, the network or an AI: callers load the data and pass it in.
// See plan/ai/tools/career/step-129-manual-job-match.md for the model.
//
// Every number that shapes a score is in this file, so tuning (step 7 of the plan) is a one-line change.
package jobmatcher

// EngineVersion is recorded with every result, so a score can be traced to the constants that made it.
const EngineVersion = "manual-1"

// The five components and their weights; they add up to 100.
const (
	WeightSkills     = 40
	WeightTitle      = 25
	WeightExperience = 15
	WeightLocation   = 10
	WeightLevel      = 10
)

// Skills, text mode: the saturating curve and the weight of one hit.
const (
	SkillSaturation = 4.0  // skills = 1 - exp(-sum(w) / SkillSaturation)
	SkillWeightMin  = 0.5  // the weight of a skill that is in every saved job
	SkillWeightMax  = 1.5  // the cap, with the title bonus
	SkillTitleBonus = 0.25 // added when the skill is in the job's title
	CorpusMinJobs   = 20   // under this many saved jobs every skill weighs 1
)

// Skills, requirements mode: how strongly each kind of persona evidence covers a requirement.
const (
	EvidenceSkill      = 1.0
	EvidenceExperience = 0.7
	EvidenceOther      = 0.5 // headline, summary, desired titles
	LongRequirement    = 4   // a requirement of this many words or more is matched by its words, not as a phrase
	CoverThreshold     = 0.6 // the share of a long requirement's significant words that must be found
)

// Title fit.
const (
	TitleWeightDesired    = 1.0
	TitleWeightHeadline   = 0.85
	TitleWeightExperience = 0.75
	PartialWordMatch      = 0.7 // a long compound that ends with the other word
	StemMatch             = 0.7 // the same stem, different endings
)

// A persona title that is entirely contained in the job's title ("Software Engineer" in "Senior Software Engineer")
// is a near-perfect fit even though the job adds words; RelevantTitleSim is how alike an experience entry's title
// must be to the job's for the entry to count as similar work.
const (
	ContainedTitleSim = 0.95
	RelevantTitleSim  = 0.5
)

// Experience.
const (
	DefaultRequiredYears = 3
	MinRequiredYears     = 1
	MaxRequiredYears     = 15
)

// Location. LocationRemote is the old reading for a persona that has not set a remote preference: the job is remote or
// hybrid and the places differ. The two below belong to an explicit preference (step 130): a hybrid job for a persona
// that is remote only, and a hybrid job in another city for a persona that is open to remote or hybrid.
const (
	LocationRemote              = 0.7
	LocationRemoteOnlyHybrid    = 0.3
	LocationOpenHybridElsewhere = 0.5
)

// Level: months of experience below which a persona is junior (0) or mid (1); more is senior (2).
const (
	JuniorBelowMonths = 24
	MidBelowMonths    = 72
)

// Combining: score = round(base * (ConfidenceBase + ConfidenceSpan * coverage)). Calibrated against real jobs (step 7): a
// job with only a title to go on must not outrank a fully described one, so the factor falls from 1 (everything
// judged) to 0.5 (nothing but a sliver).
const (
	ConfidenceBase  = 0.5
	ConfidenceSpan  = 0.5
	LowConfidenceBy = 0.5 // coverage under this marks the result as low confidence
)

// The relevance gate (step 7): where the job is, and how senior it is, only mean something when the job is in the
// persona's field. When Skills and Title fit together earn fewer than RelevanceFloor points, the whole score is scaled
// down in proportion, so an unrelated job in the same city at the same level stays low.
const RelevanceFloor = 12.0

// A job counts as on-site only when its text is long enough to have said so if it were not: at least this many
// characters of description. A job with a place but little or no text is of unknown work mode (calibration, step 130:
// on the real jobs many have no description at all, and reading those as on-site would halve the score of every one for
// a remote-only persona, on a guess).
const WorkModeMinText = 150

// The dealbreaker scale (step 130): an explicit remote preference that rules the job out (a remote-only persona and an
// on-site job, an on-site-only persona and a remote job) multiplies the whole score by this, on top of the 10 points of
// the Location component, because such a choice is meant as a filter. A hybrid job is never ruled out, and a place the
// persona does not want is not a dealbreaker (there is no notion of relocation).
const DealbreakerScale = 0.5

// Lists in a result.
const (
	MaxGaps        = 8
	MaxSuggestions = 12
)

// Input caps, so a huge record cannot make a run slow.
const (
	MaxDescriptionRunes = 20000
	MaxSkills           = 500
	MaxExperience       = 100
	MaxRequirements     = 30
)
