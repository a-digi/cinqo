// list_links.go (Namako step 4) — POST /list-links: open a page and return the addresses behind the elements a person's CSS selectors name,
// plus the address of the "next page" link. It is how a rule set's listing.linkSelectors / listing.pagination are applied to a rendered page.
// Read-only: the page is only read (one fixed script), nothing is typed or clicked.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Limits of /list-links.
const (
	maxListSelectors   = 20
	maxListSelectorLen = 200
	maxListLinks       = 500
)

type listLinksRequest struct {
	URL          string   `json:"url"`
	Selectors    []string `json:"selectors"`
	NextSelector string   `json:"nextSelector,omitempty"`
	WaitMs       int      `json:"waitMs,omitempty"`
	RequestID    string   `json:"requestId,omitempty"`
}

// ListLinksResult is the answer of /list-links.
type ListLinksResult struct {
	URL      string   `json:"url"`
	FinalURL string   `json:"finalUrl"`
	Links    []string `json:"links"`
	// Matched is the number of elements the selectors matched (some may have no address).
	Matched int `json:"matched"`
	// NextURL is the address of the next-page link, "" when there is none.
	NextURL   string `json:"nextUrl"`
	Truncated bool   `json:"truncated"`
}

func validListSelector(s string) bool {
	if strings.TrimSpace(s) == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxListSelectorLen {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validTextSelector: empty (not asked for) or a selector of at most maxListSelectorLen characters without control characters.
func validTextSelector(s string) bool { return s == "" || validListSelector(s) }

func (r listLinksRequest) check() error {
	if strings.TrimSpace(r.URL) == "" {
		return errors.New("url is required")
	}
	if len(r.Selectors) == 0 || len(r.Selectors) > maxListSelectors {
		return fmt.Errorf("selectors: from 1 to %d are required", maxListSelectors)
	}
	for _, s := range r.Selectors {
		if !validListSelector(s) {
			return errors.New("a selector is empty, too long or contains control characters")
		}
	}
	if r.NextSelector != "" && !validListSelector(r.NextSelector) {
		return errors.New("nextSelector is too long or contains control characters")
	}
	if r.WaitMs < 0 {
		return errors.New("waitMs must not be negative")
	}
	return nil
}

// listLinksJS reads the page. The selectors travel as JSON string literals (never concatenated as code). A selector the page cannot parse is
// reported in `bad` instead of failing the whole read.
func listLinksJS(selectors []string, next string) (string, error) {
	sel, err := json.Marshal(selectors)
	if err != nil {
		return "", err
	}
	nx, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	return `(function(){
  var sels = ` + string(sel) + `, nextSel = ` + string(nx) + `;
  function hrefOf(el){
    if (!el) return '';
    var a = el.tagName === 'A' ? el : (el.closest ? el.closest('a') : null) || (el.querySelector ? el.querySelector('a') : null);
    return a && a.href ? String(a.href) : '';
  }
  var out = {links: [], matched: 0, next: '', bad: []};
  for (var i = 0; i < sels.length; i++) {
    var nodes;
    try { nodes = document.querySelectorAll(sels[i]); } catch (e) { out.bad.push(sels[i]); continue; }
    out.matched += nodes.length;
    for (var j = 0; j < nodes.length; j++) { var h = hrefOf(nodes[j]); if (h) out.links.push(h); }
  }
  if (nextSel) {
    try { out.next = hrefOf(document.querySelector(nextSel)); } catch (e) { out.bad.push(nextSel); }
  }
  return JSON.stringify(out);
})()`, nil
}

type listLinksPage struct {
	Links   []string `json:"links"`
	Matched int      `json:"matched"`
	Next    string   `json:"next"`
	Bad     []string `json:"bad"`
}

// cleanListLinks keeps only addresses a person could open (http/https), resolved, each once, at most maxListLinks.
func cleanListLinks(base string, raw []string) (links []string, truncated bool) {
	b, _ := url.Parse(base)
	seen := map[string]bool{}
	links = []string{}
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l == "" || len(l) > 2000 {
			continue
		}
		u, err := url.Parse(l)
		if err != nil {
			continue
		}
		if b != nil {
			u = b.ResolveReference(u)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			continue
		}
		u.Fragment = ""
		s := u.String()
		if seen[s] {
			continue
		}
		if len(links) >= maxListLinks {
			return links, true
		}
		seen[s] = true
		links = append(links, s)
	}
	return links, false
}

// ListLinksHandler handles POST /list-links.
func ListLinksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req listLinksRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "the body must be {url, selectors, nextSelector?, waitMs?, requestId?}", http.StatusBadRequest)
		return
	}
	if err := req.check(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := runEvidence(r.Context(), req.RequestID, func(ctx context.Context) (any, error) {
		return listLinks(ctx, req)
	})
	var bad *badSelectorError
	if errors.As(err, &bad) {
		http.Error(w, bad.Error(), http.StatusBadRequest)
		return
	}
	writeEvidenceResult(w, req.RequestID, res, err, "list links")
}

type badSelectorError struct{ selectors []string }

func (e *badSelectorError) Error() string {
	return "the page could not use these selectors: " + strings.Join(e.selectors, ", ")
}

func listLinks(ctx context.Context, req listLinksRequest) (ListLinksResult, error) {
	script, err := listLinksJS(req.Selectors, req.NextSelector)
	if err != nil {
		return ListLinksResult{}, err
	}
	ep, err := openEvidencePage(ctx, req.URL, req.RequestID)
	if err != nil {
		return ListLinksResult{}, err
	}
	defer ep.Close()
	if err := sleepCtx(ctx, time.Duration(clampInt(req.WaitMs, defaultFramesWait, maxFramesWait))*time.Millisecond); err != nil {
		return ListLinksResult{}, err
	}
	text, err := rodEvalString(ep.P, script)
	if err != nil {
		return ListLinksResult{}, rodCtxOr(ctx, fmt.Errorf("rod: could not read the links: %w", err))
	}
	var page listLinksPage
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		return ListLinksResult{}, fmt.Errorf("rod: the links could not be decoded: %w", err)
	}
	if len(page.Bad) > 0 {
		return ListLinksResult{}, &badSelectorError{selectors: page.Bad}
	}
	final := req.URL
	if info, ierr := ep.P.Info(); ierr == nil && info != nil {
		final = info.URL
	}
	links, truncated := cleanListLinks(final, page.Links)
	next := ""
	if nl, _ := cleanListLinks(final, []string{page.Next}); len(nl) == 1 {
		next = nl[0]
	}
	return ListLinksResult{URL: req.URL, FinalURL: final, Links: links, Matched: page.Matched, NextURL: next, Truncated: truncated}, nil
}
