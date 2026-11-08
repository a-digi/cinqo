// detail_candidates.go (plan 186, step 2) — POST /fetch-detail-candidates: where could the title, the company, the
// location, the posted date and the description be on ONE job page? For Career's "pick the data of a job page" screen.
//
// It reads, it never ranks: the page is analysed by detail_candidates.js on the live tab (raw candidates with features,
// the structured data the page states, a flat list of its text elements) and Career ranks them (decision D6). The page is
// untrusted, so everything is bounded again here before it leaves this tool.
//
// Decision D1 (plan 186, step 0): this route is NOT gated on Katana. It reads one page, nothing on it is requested or
// clicked, and the person verifies with the real extraction before saving; the page is loaded by the Katana engine's own
// Chrome (runKatanaPage: stealth, the same verdicts, the redirect-to-private refusal) whichever engine is selected, which
// keeps the engine bookkeeping (and the idle shutdown of that Chrome) right through beginEngineFetch.
//
// Read-only and one page: no login window, no AI context (the body takes only url, requestId and rateLimitKey; any other
// field is a 400). Never an MCP tool.
package crawler

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-rod/rod"
)

//go:embed detail_candidates.js
var detailCandidatesJS string

// The bounds on what one page may return.
const (
	maxDetailCandidates = 8
	maxDetailElements   = 300
	maxDetailText       = 200
	maxDetailSelector   = 300
	maxDetailJobs       = 3
	maxDetailJSONBytes  = 4 << 20
)

type detailFurniture struct {
	Selector string `json:"selector"`
	Unique   bool   `json:"unique"`
	Kind     string `json:"kind"`
	Tag      string `json:"tag"`
	Chars    int    `json:"chars"`
	Value    string `json:"value"`
}

type detailCandidate struct {
	Selector  string `json:"selector"`
	Unique    bool   `json:"unique"`
	Matches   int    `json:"matches"`
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Chars     int    `json:"chars"`
	Tag       string `json:"tag"`
	Reason    string `json:"reason"`
	InMain    bool   `json:"inMain"`
	// PlacedIn names the page furniture the element sits in (nav, header, footer, aside, consent), "" when none.
	PlacedIn string `json:"placedIn"`
	Order    int    `json:"order"`
	Source   string `json:"source"`
	// Description candidates only.
	ShareOfPage float64           `json:"shareOfPage,omitempty"`
	ByAttr      bool              `json:"byAttr,omitempty"`
	Headings    int               `json:"headings,omitempty"`
	ListItems   int               `json:"listItems,omitempty"`
	Paragraphs  int               `json:"paragraphs,omitempty"`
	Furniture   []detailFurniture `json:"furniture,omitempty"`
	LineSignals []string          `json:"lineSignals,omitempty"`
}

type detailJobPosting struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	Company          string `json:"company"`
	Location         string `json:"location"`
	DatePosted       string `json:"datePosted"`
	DescriptionChars int    `json:"descriptionChars"`
	DescriptionPrev  string `json:"descriptionPreview"`
}

type detailMeta struct {
	OgTitle      string `json:"ogTitle"`
	OgSiteName   string `json:"ogSiteName"`
	OgDescrip    string `json:"ogDescription"`
	TwitterTitle string `json:"twitterTitle"`
}

type detailStructured struct {
	JSONLD    []detailJobPosting `json:"jsonld"`
	Meta      detailMeta         `json:"meta"`
	Microdata int                `json:"microdata"`
}

type detailElement struct {
	Selector string `json:"selector"`
	Unique   bool   `json:"unique"`
	Tag      string `json:"tag"`
	Text     string `json:"text"`
	Chars    int    `json:"chars"`
}

type detailCandidateSet struct {
	Title       []detailCandidate `json:"title"`
	Company     []detailCandidate `json:"company"`
	Location    []detailCandidate `json:"location"`
	PostedAt    []detailCandidate `json:"postedAt"`
	Description []detailCandidate `json:"description"`
}

type fetchDetailCandidatesResponse struct {
	FinalURL      string             `json:"finalUrl"`
	Title         string             `json:"title"`
	Candidates    detailCandidateSet `json:"candidates"`
	Structured    detailStructured   `json:"structured"`
	Elements      []detailElement    `json:"elements"`
	PageTextChars int                `json:"pageTextChars"`
	Truncated     bool               `json:"truncated"`
}

// cleanDetailText bounds a string from the page: no control characters, at most n runes.
func cleanDetailText(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func nonNeg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func clampCandidates(in []detailCandidate, truncated *bool) []detailCandidate {
	out := make([]detailCandidate, 0, len(in))
	for _, c := range in {
		if len(out) >= maxDetailCandidates {
			*truncated = true
			break
		}
		sel := cleanDetailText(c.Selector, maxDetailSelector+1)
		if sel == "" || utf8.RuneCountInString(sel) > maxDetailSelector {
			continue // a selector that cannot be stored is no candidate
		}
		furn := make([]detailFurniture, 0, len(c.Furniture))
		for _, f := range c.Furniture {
			if len(furn) >= maxDetailCandidates {
				break
			}
			fs := cleanDetailText(f.Selector, maxDetailSelector+1)
			if fs == "" || utf8.RuneCountInString(fs) > maxDetailSelector {
				continue
			}
			furn = append(furn, detailFurniture{Selector: fs, Unique: f.Unique, Kind: cleanDetailText(f.Kind, 30), Tag: cleanDetailText(f.Tag, 20), Chars: nonNeg(f.Chars), Value: cleanDetailText(f.Value, maxDetailText)})
		}
		signals := make([]string, 0, len(c.LineSignals))
		for _, l := range c.LineSignals {
			if len(signals) >= 10 {
				break
			}
			signals = append(signals, cleanDetailText(l, 30))
		}
		share := c.ShareOfPage
		if share < 0 || share > 1 {
			share = 0
		}
		src := c.Source
		if src != "dom" {
			src = "dom"
		}
		out = append(out, detailCandidate{
			Selector: sel, Unique: c.Unique, Matches: nonNeg(c.Matches), Attribute: cleanDetailText(c.Attribute, 40),
			Value: cleanDetailText(c.Value, maxDetailText), Chars: nonNeg(c.Chars), Tag: cleanDetailText(c.Tag, 20),
			Reason: cleanDetailText(c.Reason, 60), InMain: c.InMain, PlacedIn: cleanDetailText(c.PlacedIn, 20), Order: c.Order,
			Source: src, ShareOfPage: share, ByAttr: c.ByAttr, Headings: nonNeg(c.Headings), ListItems: nonNeg(c.ListItems),
			Paragraphs: nonNeg(c.Paragraphs), Furniture: furn, LineSignals: signals,
		})
	}
	return out
}

// clampDetailCandidates bounds everything a page returned (the page is untrusted).
func clampDetailCandidates(in fetchDetailCandidatesResponse) fetchDetailCandidatesResponse {
	truncated := in.Truncated
	out := fetchDetailCandidatesResponse{
		FinalURL:      in.FinalURL,
		Title:         cleanDetailText(in.Title, maxDetailText),
		PageTextChars: nonNeg(in.PageTextChars),
	}
	out.Candidates = detailCandidateSet{
		Title:       clampCandidates(in.Candidates.Title, &truncated),
		Company:     clampCandidates(in.Candidates.Company, &truncated),
		Location:    clampCandidates(in.Candidates.Location, &truncated),
		PostedAt:    clampCandidates(in.Candidates.PostedAt, &truncated),
		Description: clampCandidates(in.Candidates.Description, &truncated),
	}
	out.Structured.JSONLD = make([]detailJobPosting, 0, len(in.Structured.JSONLD))
	for _, j := range in.Structured.JSONLD {
		if len(out.Structured.JSONLD) >= maxDetailJobs {
			truncated = true
			break
		}
		out.Structured.JSONLD = append(out.Structured.JSONLD, detailJobPosting{
			Type: "JobPosting", Title: cleanDetailText(j.Title, maxDetailText), Company: cleanDetailText(j.Company, maxDetailText),
			Location: cleanDetailText(j.Location, maxDetailText), DatePosted: cleanDetailText(j.DatePosted, 60),
			DescriptionChars: nonNeg(j.DescriptionChars), DescriptionPrev: cleanDetailText(j.DescriptionPrev, maxDetailText),
		})
	}
	m := in.Structured.Meta
	out.Structured.Meta = detailMeta{
		OgTitle: cleanDetailText(m.OgTitle, maxDetailText), OgSiteName: cleanDetailText(m.OgSiteName, maxDetailText),
		OgDescrip: cleanDetailText(m.OgDescrip, maxDetailText), TwitterTitle: cleanDetailText(m.TwitterTitle, maxDetailText),
	}
	out.Structured.Microdata = nonNeg(in.Structured.Microdata)
	out.Elements = make([]detailElement, 0, len(in.Elements))
	for _, e := range in.Elements {
		if len(out.Elements) >= maxDetailElements {
			truncated = true
			break
		}
		sel := cleanDetailText(e.Selector, maxDetailSelector+1)
		if sel == "" || utf8.RuneCountInString(sel) > maxDetailSelector {
			continue
		}
		out.Elements = append(out.Elements, detailElement{Selector: sel, Unique: e.Unique, Tag: cleanDetailText(e.Tag, 20), Text: cleanDetailText(e.Text, maxDetailText), Chars: nonNeg(e.Chars)})
	}
	out.Truncated = truncated
	return out
}

// readDetailCandidates runs the analysis on an already loaded page (tests run it on their own page).
func decodeDetailCandidates(listJSON string) (fetchDetailCandidatesResponse, error) {
	if len(listJSON) > maxDetailJSONBytes {
		return fetchDetailCandidatesResponse{}, errors.New("the page analysis is larger than the limit")
	}
	var raw fetchDetailCandidatesResponse
	if err := json.Unmarshal([]byte(listJSON), &raw); err != nil {
		return fetchDetailCandidatesResponse{}, fmt.Errorf("the page analysis could not be decoded: %w", err)
	}
	return clampDetailCandidates(raw), nil
}

func fetchDetailCandidatesWithKatana(ctx context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error) {
	return runKatanaPage(ctx, rawURL, requestID, func(fctx context.Context, page *rod.Page, main *mainDocumentResponse) (fetchDetailCandidatesResponse, error) {
		p := page.Context(fctx)
		if err := katanaVerdict(fctx, p, rawURL, requestID, main, nil); err != nil {
			return fetchDetailCandidatesResponse{}, err
		}
		if err := waitForDOMToSettle(fctx, p); err != nil {
			return fetchDetailCandidatesResponse{}, err
		}
		title, err := rodEvalString(p, `document.title`)
		if err != nil {
			return fetchDetailCandidatesResponse{}, fmt.Errorf("katana: failed to read the title: %w", err)
		}
		finalURL, err := rodEvalString(p, `document.location.toString()`)
		if err != nil {
			return fetchDetailCandidatesResponse{}, fmt.Errorf("katana: failed to read the final URL: %w", err)
		}
		if rerr := refuseRedirectToPrivate(rawURL, finalURL); rerr != nil {
			return fetchDetailCandidatesResponse{}, rerr
		}
		listJSON, err := rodEvalString(p, detailCandidatesJS)
		if err != nil {
			return fetchDetailCandidatesResponse{}, fmt.Errorf("katana: failed to analyse the page: %w", err)
		}
		res, err := decodeDetailCandidates(listJSON)
		if err != nil {
			return fetchDetailCandidatesResponse{}, fmt.Errorf("katana: %w", err)
		}
		res.FinalURL = finalURL
		res.Title = cleanDetailText(title, maxDetailText)
		return res, nil
	})
}

// fetchDetailCandidatesFn is the fetch behind a variable so tests can script it without a browser.
var fetchDetailCandidatesFn = fetchDetailCandidatesWithKatana

// fetchDetailCandidatesRequest is the body of POST /fetch-detail-candidates; unknown fields are refused.
type fetchDetailCandidatesRequest struct {
	URL          string `json:"url"`
	RequestID    string `json:"requestId,omitempty"`
	RateLimitKey string `json:"rateLimitKey,omitempty"`
	// LoginSessionID (plan 195): the kept login to fetch with; see login_session_fetch.go. Never set by an AI tool.
	LoginSessionID string `json:"loginSessionId,omitempty"`
	// Attended (plan 198): a person started this and is watching, so a check only they can clear may open the Katana window. Never set by an AI tool on its own.
	Attended bool `json:"attended,omitempty"`
}

// FetchDetailCandidatesHandler handles POST /fetch-detail-candidates.
func FetchDetailCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body fetchDetailCandidatesRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		http.Error(w, "url is required (and only url, requestId, rateLimitKey, loginSessionId and attended are accepted)", http.StatusBadRequest)
		return
	}
	if err := checkLoginSessionID(body.LoginSessionID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(body.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancel(withAttended(withLoginSession(r.Context(), body.LoginSessionID), body.Attended))
	registerCrawlCancel(body.RequestID, cancel)
	defer func() {
		unregisterCrawlCancel(body.RequestID)
		cancel()
	}()
	defer beginEngineFetch(fetchEngineKatana)() // the Katana Chrome is stopped after its idle time like for every fetch
	awaitPortalCrawlPacing(ctx, body.RateLimitKey)

	result, err := fetchDetailCandidatesFn(ctx, body.URL, body.RequestID)
	notePacingOutcome(body.RateLimitKey, err)
	withPacingRetry(body.RateLimitKey, err)
	if err != nil {
		setCrawlPhase(body.RequestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			writeFetchLinksJSON(w, http.StatusBadGateway, cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("fetch detail candidates failed: %v", err), http.StatusBadGateway)
		return
	}
	setCrawlPhase(body.RequestID, phaseCompleted, "page analysed")
	writeFetchLinksJSON(w, http.StatusOK, result)
}
