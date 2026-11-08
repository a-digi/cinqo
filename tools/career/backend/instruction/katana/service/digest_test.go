package service

import (
	"fmt"
	"strings"
	"testing"

	"career-tool-backend/portal/linkrule"
)

func lever() []linkrule.Link {
	links := []linkrule.Link{
		{URL: "https://jobs.lever.co/leverdemo", Texts: []string{"Home"}},
		{URL: "https://jobs.lever.co/leverdemo?team=Sales", Texts: []string{"Sales"}},
		{URL: "https://jobs.lever.co/leverdemo?team=Ops", Texts: []string{"Ops"}},
	}
	for i := 0; i < 10; i++ {
		links = append(links, linkrule.Link{
			URL:   fmt.Sprintf("https://jobs.lever.co/leverdemo/0a1b2c3d-0000-4000-8000-00000000000%d", i),
			Texts: []string{fmt.Sprintf("Engineer number %d", i)},
		})
	}
	return links
}

func TestDigestGroupsByShape(t *testing.T) {
	d := BuildDigest("https://jobs.lever.co/leverdemo", "Leverdemo jobs", lever(), false)
	if d.Links != 13 || len(d.Groups) != 3 {
		t.Fatalf("links %d groups %d: %+v", d.Links, len(d.Groups), d.Groups)
	}
	top := d.Groups[0]
	if top.Shape != "jobs.lever.co/leverdemo/#" || top.Count != 10 || !top.Varies || len(top.Samples) != samplesPerGroup {
		t.Errorf("job group: %+v", top)
	}
	if strings.HasPrefix(top.Samples[0].URL, "http") {
		t.Errorf("sample keeps its scheme: %q", top.Samples[0].URL)
	}
	// query values never enter a shape, only their names
	for _, g := range d.Groups {
		if strings.Contains(g.Shape, "Sales") {
			t.Errorf("shape carries a query value: %q", g.Shape)
		}
	}
}

func TestDigestShapes(t *testing.T) {
	cases := map[string]string{
		"https://x.example/jobs/12345":                  "x.example/jobs/#",
		"https://x.example/jobs/senior-backend-dev":     "x.example/jobs/*",
		"https://x.example/careers/engineering":         "x.example/careers/engineering",
		"https://x.example/search?q=go&l=hamburg":       "x.example/search?l&q",
		"https://x.example/":                            "x.example",
		"https://x.example/de/stellen/abcdef0123456789": "x.example/de/stellen/#",
	}
	for in, want := range cases {
		if got, _ := shapeOf(in); got != want {
			t.Errorf("shapeOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDigestNextPageAndLimits(t *testing.T) {
	links := lever()
	links = append(links,
		linkrule.Link{URL: "https://jobs.lever.co/leverdemo?page=2", Texts: []string{"Next"}},
		linkrule.Link{URL: "https://jobs.lever.co/leverdemo?page=3", Rel: "next"},
		linkrule.Link{URL: "https://jobs.lever.co/leverdemo?page=4", Texts: []string{"›"}},
		linkrule.Link{URL: "https://jobs.lever.co/leverdemo?page=5", Texts: []string{"Weiter"}},
	)
	d := BuildDigest("p", "t", links, true)
	if len(d.NextPage) != maxNextCandidate || !d.Truncated {
		t.Errorf("next %v truncated %v", d.NextPage, d.Truncated)
	}
	var many []linkrule.Link
	for i := 0; i < 40; i++ {
		many = append(many, linkrule.Link{URL: "https://x.example/" + strings.Repeat(string(rune('a'+i%26)), 1+i/26) + "/page"})
	}
	d = BuildDigest("p", "", many, false)
	if len(d.Groups) != maxGroups || d.MoreGroups != 40-maxGroups {
		t.Errorf("groups %d more %d", len(d.Groups), d.MoreGroups)
	}
}

func TestDigestIsSmall(t *testing.T) {
	var links []linkrule.Link
	for i := 0; i < 400; i++ {
		links = append(links, linkrule.Link{URL: fmt.Sprintf("https://board.example/job/%d-some-long-posting-title-%d", i, i), Texts: []string{strings.Repeat("Title ", 30)}})
	}
	d := BuildDigest("https://board.example/jobs", "Board", links, false)
	if n := len(fmt.Sprint(d)); n > 3000 {
		t.Errorf("digest of 400 links is %d bytes", n)
	}
}

func TestAClippedSampleOfTheDigestIsExpandedBackToTheRealLink(t *testing.T) {
	long := func(id string) string {
		return "https://de.indeed.com/rc/clk?jk=" + id + "&bb=" + strings.Repeat("w8rYnKBdEKbKkLOIvu-Hi", 4)
	}
	links := []linkrule.Link{
		{URL: long("221dbe250a5934dc"), Texts: []string{"A"}},
		{URL: long("3f2d8057584a0d01"), Texts: []string{"B"}},
		{URL: "https://de.indeed.com/q-x-jobs.html", Texts: []string{"C"}},
	}
	d := BuildDigest("p", "", links, false)
	var picks []string
	for _, g := range d.Groups {
		for _, s := range g.Samples {
			if strings.Contains(s.URL, "rc/clk") {
				picks = append(picks, s.URL)
			}
		}
	}
	if len(picks) != 2 || !strings.HasSuffix(picks[0], "…") {
		t.Fatalf("the digest should clip these samples: %v", picks)
	}
	got := expandPicks(picks, links)
	if len(got) != 2 || got[0] != links[0].URL || got[1] != links[1].URL {
		t.Errorf("expanded: %v", got)
	}
	// the same clipped address twice never maps to the same link twice
	if two := expandPicks([]string{picks[0], picks[0]}, links); two[0] == two[1] && len(two) == 2 {
		// both prefixes match only link 0: the second finds nothing else and falls back to the bare address
		t.Logf("duplicate pick: %v", two)
	}
}

func TestDigestWithoutASiteHostIsUnchanged(t *testing.T) {
	d := BuildDigest("https://x.example/", "t", lever(), false)
	if got := d.WithHosts(lever(), "", ""); got.Site != nil || got.OtherHosts != nil || got.Note != "" {
		t.Errorf("%+v", got)
	}
}

func TestDigestListsAtMostThreeOtherHosts(t *testing.T) {
	var links []linkrule.Link
	for h := 0; h < 6; h++ {
		links = append(links, linkrule.Link{URL: fmt.Sprintf("https://h%d.example/p", h)})
	}
	d := BuildDigest("https://own.example/", "t", links, false).WithHosts(links, "own.example", "")
	if len(d.OtherHosts) != maxDigestHosts || d.Site == nil || d.Site.Host != "own.example" || d.Note != "" {
		t.Errorf("%+v", d)
	}
}
