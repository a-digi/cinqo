package deletedjobs

import (
	"strings"
	"testing"
)

// Real URLs from the job database (StepStone and duapune), the shapes the crawler actually produces.
var realURLs = []string{
	"https://www.stepstone.de/stellenangebote--Head-of-Software-Engineering-E-Commerce-m-w-d-Hamburg-freenet-DLS-GmbH--14612706-inline.html",
	"https://www.stepstone.de/stellenangebote--Junior-Software-Engineer-m-w-d-Greenfield-InsurTech-Hamburg-CHECK24--14478103-inline.html",
	"https://www.stepstone.de/stellenangebote--Senior-Software-Engineer-Java-all-genders-Hamburg-adesso-health-solutions-GmbH--13949106-inline.html",
	"https://www.stepstone.de/stellenangebote--Software-Engineer-Smalltalk-all-genders-Hamburg-adesso-insurance-solutions-GmbH--14049775-inline.html",
	"https://duapune.com/jobs/122247-inxhinier-ndertimi-projekte-energjetike",
}

func TestTheNormalizedFormOfARealURL(t *testing.T) {
	cases := map[string]string{
		realURLs[0]: "stepstone.de/stellenangebote--head-of-software-engineering-e-commerce-m-w-d-hamburg-freenet-dls-gmbh--14612706-inline.html",
		realURLs[4]: "duapune.com/jobs/122247-inxhinier-ndertimi-projekte-energjetike",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q)\n got  %q\n want %q", in, got, want)
		}
	}
}

// The same page written differently has one hash: scheme, www, case, trailing and doubled slashes, a fragment, a
// default port, user info, tracking parameters in any order, a dot segment. Tested on each real URL.
func TestTheSamePageHasOneHash(t *testing.T) {
	for _, base := range realURLs {
		want := URLHash(base)
		if want == "" {
			t.Fatalf("no hash for %q", base)
		}
		noScheme := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
		host, rest, _ := strings.Cut(noScheme, "/")
		bare := strings.TrimPrefix(host, "www.")
		variants := map[string]string{
			"http":            "http://" + noScheme,
			"no www":          "https://" + bare + "/" + rest,
			"with www":        "https://www." + bare + "/" + rest,
			"upper host":      "https://" + strings.ToUpper(host) + "/" + rest,
			"upper path":      "https://" + host + "/" + strings.ToUpper(rest),
			"trailing slash":  base + "/",
			"doubled slash":   "https://" + host + "//" + rest,
			"fragment":        base + "#apply",
			"default port":    "https://" + host + ":443/" + rest,
			"http port 80":    "http://" + host + ":80/" + rest,
			"user info":       "https://user:pw@" + host + "/" + rest,
			"utm":             base + "?utm_source=newsletter&utm_medium=email&utm_campaign=x",
			"click ids":       base + "?gclid=abc&fbclid=def&msclkid=ghi",
			"referrer":        base + "?ref=home&referrer=x&source=y&src=z&trk=t&campaign=c&mc_cid=1&mc_eid=2",
			"dot segment":     "https://" + host + "/./" + rest,
			"parent segment":  "https://" + host + "/x/../" + rest,
			"trailing dot":    "https://" + host + "./" + rest,
			"surrounding gap": "  " + base + " \n",
			"mixed":           "HTTP://WWW." + strings.ToUpper(bare) + ":80//" + rest + "/?UTM_Source=a#top",
		}
		for name, v := range variants {
			if got := URLHash(v); got != want {
				t.Errorf("%s: %q\n hash %s\n want %s\n norm %q vs %q", name, v, got, want, NormalizeURL(v), NormalizeURL(base))
			}
		}
	}
}

// Different pages never share a hash: every real URL, and small changes that matter.
func TestDifferentPagesHaveDifferentHashes(t *testing.T) {
	seen := map[string]string{}
	all := append([]string{}, realURLs...)
	all = append(all,
		"https://www.stepstone.de/jobs/software-engineer",
		"https://www.stepstone.de/jobs/software-engineers",
		"https://example.com/jobs?id=1",
		"https://example.com/jobs?id=2",
		"https://example.com/jobs?id=1&page=2",
		"https://example.com/jobs?page=2&id=1&x=",
		"https://example.com:8080/jobs",
		"https://example.com/jobs",
		"https://other.example.com/jobs",
		"https://example.org/jobs",
		"https://example.com/jobs/a",
		"https://example.com/jobs/a/b",
		"https://sub.example.com/x",
		"https://wwwexample.com/x",
		"ftp://example.com/x",
		"mailto:job@example.com",
		"/relative/path",
		"manual:abc123",
	)
	for _, u := range all {
		h := URLHash(u)
		if h == "" {
			t.Fatalf("no hash for %q", u)
		}
		if prev, dup := seen[h]; dup {
			t.Errorf("%q and %q share the hash %s", u, prev, h)
		}
		seen[h] = u
	}
}

// A parameter that identifies the page (an id, a page number) is kept; order does not matter; a bare name equals an
// empty value; the tracking names are matched in any case and as prefixes only where listed.
func TestQueryParameters(t *testing.T) {
	same := [][2]string{
		{"https://x.com/j?id=7&page=2", "https://x.com/j?page=2&id=7"},
		{"https://x.com/j?id=7", "https://x.com/j?ID=7"},
		{"https://x.com/j?token=AbC", "https://x.com/j?token=aBc"},
		{"https://x.com/j?id=7", "https://x.com/j?id=7&UTM_Source=a&Utm_Medium=b"},
		{"https://x.com/j?flag", "https://x.com/j?flag="},
		{"https://x.com/j?a=1&a=2", "https://x.com/j?a=2&a=1"},
		{"https://x.com/j?utm_source=a", "https://x.com/j"},
		{"https://x.com/j?", "https://x.com/j"},
		{"https://x.com/j?mc_anything=1", "https://x.com/j"},
	}
	for _, c := range same {
		if URLHash(c[0]) != URLHash(c[1]) {
			t.Errorf("should match: %q (%q) and %q (%q)", c[0], NormalizeURL(c[0]), c[1], NormalizeURL(c[1]))
		}
	}
	different := [][2]string{
		{"https://x.com/j?id=7", "https://x.com/j?id=8"},
		{"https://x.com/j?id=7", "https://x.com/j"},
		{"https://x.com/j?id=7&page=2", "https://x.com/j?id=7"},
		{"https://x.com/j?job=7", "https://x.com/j?id=7"},
		{"https://x.com/j?sourceid=7", "https://x.com/j"}, // "source" is tracking, "sourceid" is not
		{"https://x.com/j?reference=7", "https://x.com/j"},
		{"https://x.com/j?a=1&a=2", "https://x.com/j?a=1"},
		{"https://x.com/j?x=%zz", "https://x.com/j?x=%yy"}, // malformed queries are kept whole, never merged
	}
	for _, c := range different {
		if URLHash(c[0]) == URLHash(c[1]) {
			t.Errorf("should differ: %q and %q (both %q)", c[0], c[1], NormalizeURL(c[0]))
		}
	}
}

// Percent-encoding and Unicode: the encoded and the plain spelling of a path are one page; case of non-ASCII letters
// folds.
func TestEncodingAndUnicode(t *testing.T) {
	if URLHash("https://x.com/jobs/ärzte") != URLHash("https://x.com/jobs/%C3%A4rzte") {
		t.Error("encoded and plain umlaut differ")
	}
	if URLHash("https://x.com/jobs/ÄRZTE") != URLHash("https://x.com/jobs/ärzte") {
		t.Error("upper and lower umlaut differ")
	}
	if URLHash("https://x.com/jobs/a%20b") != URLHash("https://x.com/jobs/a b") {
		t.Error("encoded and plain space differ")
	}
	if URLHash("https://x.com/jobs/ärzte") == URLHash("https://x.com/jobs/arzte") {
		t.Error("an umlaut must not fold to its base letter")
	}
}

// Things that are not web addresses are only trimmed, stripped of the fragment and lower-cased, and never equal a web
// address; blank gives no key at all.
func TestOddInputs(t *testing.T) {
	for _, blank := range []string{"", "   ", "\n\t"} {
		if NormalizeURL(blank) != "" || URLHash(blank) != "" {
			t.Errorf("%q should have no key", blank)
		}
	}
	cases := map[string]string{
		"/relative/Path#x":         "raw:/relative/path",
		"Manual:ABC":               "raw:manual:abc",
		"mailto:Job@Example.com":   "raw:mailto:job@example.com",
		"ftp://example.com/File":   "raw:ftp://example.com/file",
		"https://":                 "raw:https://",
		"http://[::1/x":            "raw:http://[::1/x", // unparsable
		"example.com/jobs":         "raw:example.com/jobs",
		"//example.com/jobs":       "raw://example.com/jobs",
		"javascript:alert(1)":      "raw:javascript:alert(1)",
		"https://%zz.example.com/": "raw:https://%zz.example.com/",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
	if URLHash("example.com/jobs") == URLHash("https://example.com/jobs") {
		t.Error("a scheme-less string must not equal the web address")
	}
}

// Host details: ports other than the default stay, IPv6 keeps its brackets, a different host is a different page.
func TestHosts(t *testing.T) {
	cases := map[string]string{
		"https://Example.COM/a":        "example.com/a",
		"https://www.example.com/":     "example.com",
		"https://example.com":          "example.com",
		"https://example.com/":         "example.com",
		"http://example.com:8080/":     "example.com:8080",
		"https://example.com:80/a":     "example.com:80/a", // 80 is not the default of https
		"http://example.com:443/a":     "example.com:443/a",
		"https://[::1]:8443/a":         "[::1]:8443/a",
		"https://[::1]/a":              "[::1]/a",
		"https://www.www.example.com/": "www.example.com",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// The hash is SHA-256 over "url:" + the normalized text, in hex: these literal values catch any change of the
// algorithm, the prefix or the normalization of a real URL. The prefix keeps it apart from a plain hash of the text.
func TestTheHashIsStableAndPrefixed(t *testing.T) {
	golden := map[string]string{
		realURLs[0]:              "4aa048a3cc8dedeb177e07790d9cd5b081fa9b3faa9aeed0eb9f29dd08976cbb",
		realURLs[4]:              "7aec228caf56f7452fb13f16a75fca65d00bf372c807806219abc31998f85d8c",
		"https://example.com/":   "d595c01d3b9a1c6cd60266d0d29db3d43bc194021b78498a91204a849a8c89dd",
		"http://www.example.com": "d595c01d3b9a1c6cd60266d0d29db3d43bc194021b78498a91204a849a8c89dd",
	}
	for in, want := range golden {
		if got := URLHash(in); got != want {
			t.Errorf("URLHash(%q) = %s, want %s", in, got, want)
		}
	}
	if URLHash("https://example.com") == "a379a6f6eeafb9a55e378c118034e2751e682fab9f2d30ab13d2125586ce1947" {
		t.Error("the hash must not equal a plain SHA-256 of the normalized text (prefix missing)")
	}
	if len(URLHash("https://example.com")) != 64 {
		t.Error("not 64 hex characters")
	}
	if KeyVersion != 1 {
		t.Errorf("KeyVersion = %d: if the rules changed on purpose, update this test and recompute the stored hashes", KeyVersion)
	}
}

// Normalizing never panics, whatever it is given.
func TestNeverPanics(t *testing.T) {
	for _, in := range []string{"%", "%%%", "http://", "http://%", "://", ":", "http:///", "http://:80", "http://a b/c", "\x00", "http://\x7f.com", "https://[", "https://]", "?a=b", "#", "http://x/?%", "http://x/%zz?%zz#%zz"} {
		_ = NormalizeURL(in)
		_ = URLHash(in)
	}
}
