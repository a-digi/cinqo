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
