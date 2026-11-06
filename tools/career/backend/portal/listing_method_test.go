package portal

import "testing"

// Plan 190: the engine decides how a link's job list is crawled.
func TestListingMethodFollowsTheEngine(t *testing.T) {
	for _, c := range []struct {
		engine string
		login  bool
		want   string
	}{
		{"katana", false, CrawlMethodLinks},
		{" katana ", false, CrawlMethodLinks}, // surrounding space is not a different engine
		{"rod", false, CrawlMethodInstructions},
		{"", false, CrawlMethodInstructions},         // an older Browser sends none
		{"Katana", false, CrawlMethodInstructions},   // exact, not case-folded
		{"selenium", false, CrawlMethodInstructions}, // any other name
		{"katana", true, CrawlMethodInstructions},    // a login link is always read with its AI instructions
		{"rod", true, CrawlMethodInstructions},
	} {
		if got := ListingMethodFor(c.engine, c.login); got != c.want {
			t.Errorf("ListingMethodFor(%q, login=%v) = %q, want %q", c.engine, c.login, got, c.want)
		}
	}
}
