// frames.go (Namako step 1) — POST /fetch-frames: the frame tree of a page. A page that plays a video usually does so inside one or two iframes
// (often of another site); this returns every frame with its address, origin, name, the sandbox/allow attributes the parent gave it, its visible
// text (capped), the links and media addresses it holds, and the depth it sits at. Read-only.
//
// The tree comes from Chrome's DOM with iframes pierced (DOM.getDocument pierce=true), which also reaches cross-site frames; the pure builder
// (buildFrameTree) works on that node tree, so the rules (caps, text, links) are tested without a browser.
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

	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultFrameDepth  = 3
	maxFrameDepth      = 4
	defaultFrameCount  = 20
	maxFrameCount      = 30
	frameTextCap       = 4000
	frameLinksCap      = 100
	defaultFramesWait  = 1500
	maxFramesWait      = 10000
	maxRequestBodySize = 1 << 16
	maxSelectedText    = 500
)

// FrameInfo is one frame of the page.
type FrameInfo struct {
	FrameID  string   `json:"frameId"`
	ParentID string   `json:"parentId,omitempty"`
	URL      string   `json:"url"`
	Origin   string   `json:"origin,omitempty"`
	Depth    int      `json:"depth"`
	Name     string   `json:"name,omitempty"`
	Sandbox  string   `json:"sandbox,omitempty"`
	Allow    string   `json:"allow,omitempty"`
	Title    string   `json:"title,omitempty"`
	Text     string   `json:"text,omitempty"`
	Links    []string `json:"links,omitempty"`
	Media    []string `json:"mediaSources,omitempty"`
	// Unreadable: the frame's document could not be read (not loaded, blocked); only its address attribute is known.
	Unreadable bool `json:"unreadable,omitempty"`
	// TextTruncated: the frame's text was cut at the per-frame cap.
	TextTruncated bool `json:"textTruncated,omitempty"`
}

// FramesResult is the answer of /fetch-frames.
type FramesResult struct {
	URL       string      `json:"url"`
	FinalURL  string      `json:"finalUrl"`
	Frames    []FrameInfo `json:"frames"`
	Truncated bool        `json:"truncated"`
	// SelectedText is the text of the first element the request's textSelector matched ("" when none, or no selector was asked for).
	SelectedText string `json:"selectedText,omitempty"`
}

type fetchFramesRequest struct {
	URL       string `json:"url"`
	MaxDepth  int    `json:"maxDepth,omitempty"`
	MaxFrames int    `json:"maxFrames,omitempty"`
	WaitMs    int    `json:"waitMs,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	// TextSelector (Namako step 4): a CSS selector whose first match's text is returned as selectedText (the page's title element, for example).
	TextSelector string `json:"textSelector,omitempty"`
}

// clampInt returns def for a zero value, and v limited to [1, hi] otherwise; a negative value is an error for the caller to refuse.
func clampInt(v, def, hi int) int {
	switch {
	case v == 0:
		return def
	case v > hi:
		return hi
	default:
		return v
	}
}

// attrs turns a node's flat [name, value, name, value …] list into a map (lower-cased names).
func attrs(n *proto.DOMNode) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		m[strings.ToLower(n.Attributes[i])] = n.Attributes[i+1]
	}
	return m
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// cleanText collapses whitespace.
func cleanText(s string) string { return strings.Join(strings.Fields(s), " ") }

// frameCollector gathers the text, links and media addresses of ONE document without entering its child frames.
type frameCollector struct {
	base   string
	text   strings.Builder
	title  string
	links  []string
	media  []string
	seenL  map[string]bool
	seenM  map[string]bool
	cutAt  int
	cutOff bool
}

func (c *frameCollector) addLink(list *[]string, seen map[string]bool, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2000 || len(*list) >= frameLinksCap {
		return
	}
	abs := raw
	if base, err := url.Parse(c.base); err == nil {
		if ref, err := url.Parse(raw); err == nil {
			abs = base.ResolveReference(ref).String()
		}
	}
	// Only addresses a person could open: no javascript:, data:, blob:, mailto:.
	if u, err := url.Parse(abs); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return
	}
	if !seen[abs] {
		seen[abs] = true
		*list = append(*list, abs)
	}
}

// collect walks the nodes of one document; it does not descend into an iframe's ContentDocument (the caller does) and returns the iframe nodes found.
func (c *frameCollector) collect(n *proto.DOMNode, frames *[]*proto.DOMNode) {
	switch strings.ToUpper(n.NodeName) {
	case "SCRIPT", "STYLE", "NOSCRIPT", "TEMPLATE":
		return
	case "#TEXT":
		if c.text.Len() < c.cutAt {
			if t := cleanText(n.NodeValue); t != "" {
				if c.text.Len() > 0 {
					c.text.WriteByte(' ')
				}
				c.text.WriteString(t)
			}
		} else {
			c.cutOff = true
		}
		return
	case "TITLE":
		for _, ch := range n.Children {
			if strings.EqualFold(ch.NodeName, "#text") && c.title == "" {
				c.title = cleanText(ch.NodeValue)
			}
		}
	case "A", "AREA":
		c.addLink(&c.links, c.seenL, attrs(n)["href"])
	case "IFRAME", "FRAME":
		*frames = append(*frames, n)
		c.addLink(&c.links, c.seenL, attrs(n)["src"])
		return // an iframe's own content is another frame
	case "VIDEO", "AUDIO", "SOURCE", "TRACK", "EMBED", "OBJECT":
		a := attrs(n)
		c.addLink(&c.media, c.seenM, a["src"])
		c.addLink(&c.media, c.seenM, a["data"])
	}
	for _, ch := range n.Children {
		c.collect(ch, frames)
	}
}

// buildFrameTree turns the pierced DOM of a page into its frames, parents before children, within the caps. truncated says a cap left frames out.
func buildFrameTree(root *proto.DOMNode, maxDepth, maxFrames int) (frames []FrameInfo, truncated bool) {
	if root == nil {
		return nil, false
	}
	var visit func(doc *proto.DOMNode, parentID, frameID string, depth int, meta map[string]string, unreadableURL string)
	visit = func(doc *proto.DOMNode, parentID, frameID string, depth int, meta map[string]string, unreadableURL string) {
		if len(frames) >= maxFrames {
			truncated = true
			return
		}
		info := FrameInfo{FrameID: frameID, ParentID: parentID, Depth: depth, Name: meta["name"], Sandbox: meta["sandbox"], Allow: meta["allow"]}
		if doc == nil {
			info.URL, info.Unreadable = unreadableURL, true
			info.Origin = originOf(unreadableURL)
			frames = append(frames, info)
			return
		}
		info.URL = doc.DocumentURL
		info.Origin = originOf(doc.DocumentURL)
		col := &frameCollector{base: doc.DocumentURL, cutAt: frameTextCap, seenL: map[string]bool{}, seenM: map[string]bool{}}
		var iframes []*proto.DOMNode
		for _, ch := range doc.Children {
			if info.FrameID == "" && strings.EqualFold(ch.NodeName, "HTML") {
				info.FrameID = string(ch.FrameID)
			}
			col.collect(ch, &iframes)
		}
		info.Title = col.title
		text := col.text.String()
		if len(text) > frameTextCap {
			text, col.cutOff = text[:frameTextCap], true
		}
		info.Text, info.TextTruncated = text, col.cutOff
		info.Links, info.Media = col.links, col.media
		frames = append(frames, info)
		for _, f := range iframes {
			if depth+1 > maxDepth {
				truncated = true
				continue
			}
			a := attrs(f)
			visit(f.ContentDocument, info.FrameID, string(f.FrameID), depth+1, a, a["src"])
		}
	}
	visit(root, "", "", 0, nil, "")
	return frames, truncated
}

// FetchFramesHandler handles POST /fetch-frames.
func FetchFramesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fetchFramesRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required (and only url, maxDepth, maxFrames, waitMs, textSelector and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if !validTextSelector(req.TextSelector) {
		http.Error(w, "textSelector is too long or contains control characters", http.StatusBadRequest)
		return
	}
	if req.MaxDepth < 0 || req.MaxFrames < 0 || req.WaitMs < 0 {
		http.Error(w, "maxDepth, maxFrames and waitMs must not be negative", http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := runEvidence(r.Context(), req.RequestID, func(ctx context.Context) (any, error) {
		return fetchFrames(ctx, req)
	})
	writeEvidenceResult(w, req.RequestID, res, err, "fetch frames")
}

// fetchFrames opens the page and reads its frame tree.
func fetchFrames(ctx context.Context, req fetchFramesRequest) (FramesResult, error) {
	ep, err := openEvidencePage(ctx, req.URL, req.RequestID)
	if err != nil {
		return FramesResult{}, err
	}
	defer ep.Close()
	if err := sleepCtx(ctx, time.Duration(clampInt(req.WaitMs, defaultFramesWait, maxFramesWait))*time.Millisecond); err != nil {
		return FramesResult{}, err
	}
	res, err := readFrames(ctx, ep, req.URL, clampInt(req.MaxDepth, defaultFrameDepth, maxFrameDepth), clampInt(req.MaxFrames, defaultFrameCount, maxFrameCount))
	if err != nil || req.TextSelector == "" {
		return res, err
	}
	sel, err := json.Marshal(req.TextSelector)
	if err != nil {
		return FramesResult{}, err
	}
	// the selector is a JSON string literal, never code; a selector the page cannot parse gives no text
	text, err := rodEvalString(ep.P, `(function(){try{var e=document.querySelector(`+string(sel)+`);return e?String(e.textContent||''):''}catch(x){return ''}})()`)
	if err != nil {
		return FramesResult{}, rodCtxOr(ctx, fmt.Errorf("rod: could not read the selected text: %w", err))
	}
	res.SelectedText = cleanText(text)
	if r := []rune(res.SelectedText); len(r) > maxSelectedText {
		res.SelectedText = string(r[:maxSelectedText])
	}
	return res, nil
}

// readFrames reads the frame tree of an already open page.
func readFrames(ctx context.Context, ep *evidencePage, startURL string, maxDepth, maxFrames int) (FramesResult, error) {
	depth := -1
	doc, err := proto.DOMGetDocument{Depth: &depth, Pierce: true}.Call(ep.P)
	if err != nil {
		return FramesResult{}, rodCtxOr(ctx, fmt.Errorf("rod: could not read the page's frames: %w", err))
	}
	frames, truncated := buildFrameTree(doc.Root, maxDepth, maxFrames)
	final := startURL
	if info, ierr := ep.P.Info(); ierr == nil && info != nil {
		final = info.URL
	}
	if frames == nil {
		frames = []FrameInfo{}
	}
	return FramesResult{URL: startURL, FinalURL: final, Frames: frames, Truncated: truncated}, nil
}

// runEvidence runs fn under the shared bookkeeping of the evidence routes: cancellable by requestId (crawl-cancel), ends when the caller
// disconnects, keeps the Rod engine's start/stop bookkeeping right.
func runEvidence(parent context.Context, requestID string, fn func(ctx context.Context) (any, error)) (any, error) {
	ctx, cancel := context.WithCancel(parent)
	registerCrawlCancel(requestID, cancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		cancel()
	}()
	defer beginEngineFetch(fetchEngineRod)()
	return fn(ctx)
}

func writeEvidenceResult(w http.ResponseWriter, requestID string, res any, err error, what string) {
	if err != nil {
		setCrawlPhase(requestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			writeEvidenceJSON(w, http.StatusBadGateway, cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("%s failed: %v", what, err), http.StatusBadGateway)
		return
	}
	setCrawlPhase(requestID, phaseCompleted, what+" done")
	writeEvidenceJSON(w, http.StatusOK, res)
}

func writeEvidenceJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
