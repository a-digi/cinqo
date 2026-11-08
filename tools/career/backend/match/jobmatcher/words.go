package jobmatcher

import "strings"

// stopWords are dropped when titles and free text are compared (folded forms, German and English).
var stopWords = setOf(`
der die das den dem des ein eine einen einem einer eines und oder aber mit fuer von zu zum zur im in am an auf aus bei nach
ueber unter vor als auch wir sie du ihr ihre ihren ihnen dich dein deine ist sind wird werden sein haben hat dass wie wo was
nicht nur noch sehr gern gerne bitte ab bis durch gegen ohne um sowie bzw etc z b m w d f x innen
erfahrung erfahrungen kenntnisse kenntnis faehigkeit faehigkeiten jahre jahren jahr idealerweise wuenschenswert vorteil
the a an and or of to in for with on at by from as is are be been being you we your our their this that these those it its
into over under experience knowledge skills skill years year work working ability strong good etc
`)

func setOf(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// aliases map a spelling to the one form both sides are compared in; keys and values are token sequences
// (joined by a space). Longer keys win over shorter ones.
var aliases = map[string]string{
	"js":                        "javascript",
	"ts":                        "typescript",
	"k8s":                       "kubernetes",
	"postgres":                  "postgresql",
	"golang":                    "go",
	"node":                      "nodejs",
	"reactjs":                   "react",
	"vuejs":                     "vue",
	"ki":                        "ai",
	"kuenstliche intelligenz":   "ai",
	"artificial intelligence":   "ai",
	"machine learning":          "ml",
	"ms office":                 "microsoft office",
	"ms project":                "microsoft project",
	"microsoft excel":           "excel",
	"c sharp":                   "csharp",
	"amazon web services":       "aws",
	"google cloud platform":     "gcp",
	"google cloud":              "gcp",
	"structured query language": "sql",
	"sql server":                "mssql",
	"ms sql":                    "mssql",
	"power bi":                  "powerbi",
	"ci cd":                     "cicd",
	"continuous integration":    "cicd",
	"restful":                   "rest",
	"rest api":                  "rest",
	"restful api":               "rest",
}

const maxAliasWords = 3

// fillerWords are job-ad boilerplate that is never a skill: it is not offered as a requirement and never listed as a
// gap. It is separate from stopWords on purpose: those also shape title and requirement matching, these only
// shape suggestions and gaps (folded forms).
var fillerWords = setOf(`
berufserfahrung bewerbung bewerbungen unterlagen team teams unternehmen stelle position mitarbeiter mitarbeiterin
mitarbeitende aufgaben profil angebot vorteile gehalt arbeitgeber standort bereich bereiche zusammenarbeit moeglichkeit
moeglichkeiten arbeit arbeitsplatz arbeitszeit vollzeit teilzeit wochenstunden stunden woche monat verstaerkung kollegen
kolleginnen ansprechpartner kontakt informationen bereitschaft interesse freude engagement
company role candidate candidates opportunity opportunities benefits salary application applications environment
responsibilities requirements qualifications tasks profile offer contact information
`)
