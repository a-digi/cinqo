package jobmatcher

import "testing"

func TestTokenizeKeepsSkillNamesWithSymbols(t *testing.T) {
	cases := map[string][]string{
		"C++":              {"cpp"},
		"C#":               {"csharp"},
		"C/C++":            {"c", "cpp"},
		"F#":               {"fsharp"},
		".NET":             {"dotnet"},
		"ASP.NET Core":     {"aspnet", "core"},
		"Node.js":          {"nodejs"},
		"CI/CD":            {"cicd"},
		"Visual Basic 6.0": {"visual", "basic", "6", "0"},
		"Pädagogik":        {"paedagogik"},
		"Größe":            {"groesse"},
	}
	for in, want := range cases {
		eqStrings(t, in, tokenize(in), want...)
	}
}

func TestAliasesAreAppliedLongestFirst(t *testing.T) {
	eqStrings(t, "js", tokenize("JS"), "javascript")
	eqStrings(t, "k8s", tokenize("k8s"), "kubernetes")
	eqStrings(t, "golang", tokenize("Golang"), "go")
	eqStrings(t, "ms office", tokenize("MS Office"), "microsoft", "office")
	eqStrings(t, "microsoft excel", tokenize("Microsoft Excel"), "excel")
	eqStrings(t, "KI", tokenize("Künstliche Intelligenz und KI"), "ai", "und", "ai")
	eqStrings(t, "amazon web services", tokenize("Amazon Web Services"), "aws")
}

func TestContainsPhraseMatchesWholeWordsOnly(t *testing.T) {
	text := tokenize("Wir arbeiten bei Google mit JavaScript und gutem Chef")
	for _, short := range []string{"go", "java", "c", "r", "chef "} {
		ph := tokenize(short)
		want := short == "chef "
		if got := containsPhrase(text, ph); got != want {
			t.Errorf("%q in text = %v, want %v", short, got, want)
		}
	}
	if !containsPhrase(tokenize("Erfahrung mit Go und Docker"), tokenize("Go")) {
		t.Error("go as a word must match")
	}
	if containsPhrase(tokenize("Go"), tokenize("Go Docker")) || containsPhrase(nil, tokenize("go")) || containsPhrase(tokenize("go"), nil) {
		t.Error("an empty or longer phrase never matches")
	}
}

func TestWordSim(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"erzieher", "erzieher", 1},
		{"erzieher", "erzieherin", 1},
		{"erzieherin", "erziehern", StemMatch},
		{"entwickler", "entwicklung", StemMatch},
		{"heilerzieher", "erzieherin", PartialWordMatch},
		{"developer", "developers", 1},
		{"heilerzieher", "erzieher", PartialWordMatch},
		{"softwareentwickler", "entwickler", PartialWordMatch},
		{"java", "javascript", 0},
		{"test", "testing", 0}, // under 5 characters: no prefix rule
		{"engineer", "manager", 0},
		{"software", "softwareentwickler", 0}, // a prefix more than 3 characters short is not a form of the word
		{"manager", "management", 0},
		{"go", "good", 0},
	}
	for _, c := range cases {
		if got := wordSim(c.a, c.b); got != c.want {
			t.Errorf("wordSim(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
		if got := wordSim(c.b, c.a); got != c.want {
			t.Errorf("wordSim(%q,%q) = %v, want %v (symmetry)", c.b, c.a, got, c.want)
		}
	}
}

func TestDice(t *testing.T) {
	if got := dice([]string{"backend", "engineer"}, []string{"backend", "engineer"}); got != 1 {
		t.Errorf("same = %v", got)
	}
	if got := dice([]string{"software", "engineer"}, []string{"backend", "engineer"}); got != 0.5 {
		t.Errorf("half = %v", got)
	}
	if dice(nil, []string{"a"}) != 0 || dice([]string{"a"}, nil) != 0 {
		t.Error("empty is 0")
	}
}

func TestInvisibleCharactersDoNotSplitWords(t *testing.T) {
	// a soft hyphen inside a word, as some job sites put there ("En­gineer"), and zero-width characters
	eqStrings(t, "soft hyphen", tokenize("Senior Software En\u00adgineer"), "senior", "software", "engineer")
	eqStrings(t, "zero width space", tokenize("Kuber\u200bnetes"), "kubernetes")
	eqStrings(t, "byte order mark", tokenize("\ufeffDocker"), "docker")
	eqStrings(t, "title words", titleWords("Senior Software En\u00adgineer (all genders)"), "senior", "software", "engineer")
}
