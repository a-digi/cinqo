package portal

import (
	"encoding/json"
	"strings"
	"testing"
)

// Step 162: a model that retypes a long crawl result as a string damages it in two ways only: raw line breaks
// inside text and backslashes before characters JSON has no escape for. check_crawl_extraction_result repairs those.

func parsePagesText(t *testing.T, text string) ([]CrawlResultPage, error) {
	t.Helper()
	var arg CrawlPagesArg
	raw, _ := json.Marshal(text) // the string form, as the model sends it
	err := json.Unmarshal(raw, &arg)
	return arg, err
}

func firstItem(t *testing.T, pages []CrawlResultPage) map[string]any {
	t.Helper()
	if len(pages) != 1 || len(pages[0].Items) != 1 {
		t.Fatalf("want one page with one item, got %+v", pages)
	}
	return pages[0].Items[0]
}

func TestRetypedPagesAreRepairedForEachKindOfDamage(t *testing.T) {
	for name, c := range map[string]struct {
		text string
		want string // the description after parsing
	}{
		"a raw line break inside text":        {"[{\"url\":\"u\",\"items\":[{\"title\":\"A\",\"url\":\"/a\",\"description\":\"Nordex\n22419 Hamburg\"}]}]", "Nordex\n22419 Hamburg"},
		"a raw tab and carriage return":       {"[{\"url\":\"u\",\"items\":[{\"title\":\"A\",\"url\":\"/a\",\"description\":\"a\tb\rc\"}]}]", "a\tb\rc"},
		"a control character":                 {"[{\"url\":\"u\",\"items\":[{\"title\":\"A\",\"url\":\"/a\",\"description\":\"a\x01b\"}]}]", "a\x01b"},
		"a backslash before a space":          {`[{"url":"u","items":[{"title":"A","url":"/a","description":"Nordex\ \ 22419 Hamburg"}]}]`, "Nordex  22419 Hamburg"},
		"a backslash before a letter":         {`[{"url":"u","items":[{"title":"A","url":"/a","description":"a\qb\xc"}]}]`, "aqbxc"},
		"a backslash before a short u escape": {`[{"url":"u","items":[{"title":"A","url":"/a","description":"a\u12b"}]}]`, "au12b"},
		"damage and valid escapes together":   {"[{\"url\":\"u\",\"items\":[{\"title\":\"A\",\"url\":\"/a\",\"description\":\"say \\\"hi\\\"\n\\ ok \\u00e4 \\\\\"}]}]", "say \"hi\"\n ok ä \\"},
	} {
		pages, err := parsePagesText(t, c.text)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := firstItem(t, pages)["description"]; got != c.want {
			t.Errorf("%s: description = %q, want %q", name, got, c.want)
		}
		if firstItem(t, pages)["title"] != "A" || firstItem(t, pages)["url"] != "/a" {
			t.Errorf("%s: the other fields must be untouched", name)
		}
	}
}

func TestRepairLeavesValidTextAndStructureExactlyAsItIs(t *testing.T) {
	valid := "[\n  {\"url\": \"https://x/\", \"items\": [{\"title\": \"A \\\"quoted\\\" \\\\ path \\/ \\n \\t \\u00fc\", \"url\": \"/a\"}]}\n]"
	if got := repairRetypedJSON(valid); got != valid {
		t.Errorf("valid JSON must come back byte for byte:\n%q\n%q", got, valid)
	}
	// line breaks BETWEEN tokens are not text and stay
	if got := repairRetypedJSON("[\n{\"a\":\n1}\n]"); got != "[\n{\"a\":\n1}\n]" {
		t.Errorf("whitespace outside strings must stay: %q", got)
	}
	// a valid string parses without the repair being needed: identical to the array form
	pages, err := parsePagesText(t, valid)
	if err != nil || firstItem(t, pages)["title"] != "A \"quoted\" \\ path / \n \t ü" {
		t.Errorf("valid text: %v %+v", err, pages)
	}
}

func TestDamageThatIsNotRetypingStaysAnError(t *testing.T) {
	for name, text := range map[string]string{
		"a missing bracket":    `[{"url":"u","items":[{"title":"A"}]`,
		"a missing quote":      `[{"url":"u","items":[{"title":"A,"url":"/a"}]}]`,
		"not JSON at all":      `these are the pages`,
		"a lone backslash end": `[{"url":"u"}] \`,
	} {
		_, err := parsePagesText(t, text)
		if err == nil || !strings.Contains(err.Error(), "pages was a string but not a JSON array of pages") {
			t.Errorf("%s: want the old clear error, got %v", name, err)
		}
	}
}

func TestRepairedPagesGiveTheSameReportAsTheArrayThroughTheTool(t *testing.T) {
	var items []string
	for i := 0; i < 8; i++ {
		items = append(items, `{"title":"Job `+string(rune('A'+i))+`","url":"https://x/`+string(rune('a'+i))+`"}`)
	}
	items[0] = "{\"title\":\"A\",\"url\":\"https://x/a\",\"description\":\"line one\nline two\"}" // a raw line break
	items[1] = `{"title":"B\ b","url":"https://x/b"}`                                             // a backslash before a space
	items = append(items, `{}`)                                                                   // one empty row of nine: tolerated
	damaged := `[{"url":"https://x/1","items":[` + strings.Join(items, ",") + `]}]`

	res, err := callCheck(t, checkSession(t), damaged)
	if err != nil || res.IsError {
		t.Fatalf("a damaged copy must be repaired, not rejected: %v %+v", err, res)
	}
	r := reportOf(t, res)
	if r.ItemsFound != 9 || r.ItemsExtractable != 8 || r.BlankItems != 1 || !r.OK {
		t.Errorf("report: %+v", r)
	}
}
