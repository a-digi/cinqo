// frames_after.go (Namako plan 20, step 1) — the frames a page has after its presses. A player often gets its frame address only after a press (the page loads with
// blank frames); Namako then opens that address as a page of its own. This file only LISTS: it reads Chrome's frame tree, filters it and marks what is new since the
// page was loaded. It presses and opens nothing.
package crawler

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

const (
	maxFramesAfter     = 10
	frameAfterURLMax   = 300
	frameAfterDepthMax = 3
)

// FrameAfter is one frame of the page: its address, the page it is in, how deep it is, and whether it was not there when the page had loaded.
type FrameAfter struct {
	URL       string `json:"url"`
	ParentURL string `json:"parentUrl,omitempty"`
	Depth     int    `json:"depth"`
	New       bool   `json:"new"`
}

// listFrames reads Chrome's frame tree of the open page (parents before children). A page that cannot be read gives no frames.
func listFrames(ep *evidencePage) []FrameAfter {
	tree, err := proto.PageGetFrameTree{}.Call(ep.P)
	if err != nil || tree == nil || tree.FrameTree == nil {
		return nil
	}
	var out []FrameAfter
	var walk func(t *proto.PageFrameTree, parent string, depth int)
	walk = func(t *proto.PageFrameTree, parent string, depth int) {
		if t == nil || t.Frame == nil || depth > frameAfterDepthMax {
			return
		}
		if depth > 0 {
			out = append(out, FrameAfter{URL: t.Frame.URL, ParentURL: parent, Depth: depth})
		}
		for _, c := range t.ChildFrames {
			walk(c, t.Frame.URL, depth+1)
		}
	}
	walk(tree.FrameTree, "", 0)
	return out
}

// frameAddressOK says whether a frame address is one worth opening: a web address with a host (not about:blank, a data or javascript address, a blob).
func frameAddressOK(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// arrangeFrames keeps the frames with a web address, once each, cut short and at most maxFramesAfter, marks the ones that were not in the baseline as New, and puts
// the new ones first (they are what a press made appear).
func arrangeFrames(now, baseline []FrameAfter) []FrameAfter {
	was := map[string]bool{}
	for _, f := range baseline {
		was[f.URL] = true
	}
	var fresh, old []FrameAfter
	seen := map[string]bool{}
	for _, f := range now {
		if !frameAddressOK(f.URL) || seen[f.URL] {
			continue
		}
		seen[f.URL] = true
		f.New = !was[f.URL]
		f.URL = cutText(f.URL, frameAfterURLMax)
		f.ParentURL = cutText(f.ParentURL, frameAfterURLMax)
		if f.New {
			fresh = append(fresh, f)
		} else {
			old = append(old, f)
		}
	}
	out := append(fresh, old...)
	if len(out) > maxFramesAfter {
		out = out[:maxFramesAfter]
	}
	if out == nil {
		out = []FrameAfter{}
	}
	return out
}

// validateReferer checks a referer a caller sent: empty is none; otherwise a crawl address (http or https, a public host) of a sensible length.
func validateReferer(referer string) error {
	if referer == "" {
		return nil
	}
	if len(referer) > 2000 {
		return fmt.Errorf("referer is too long")
	}
	if err := validateCrawlURL(referer); err != nil {
		return fmt.Errorf("referer: %w", err)
	}
	return nil
}
