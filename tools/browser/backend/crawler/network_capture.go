// network_capture.go (Namako step 1) — POST /capture-network: the media requests a page makes. A stream is not in the page's HTML: the player
// asks for a manifest (.m3u8 / .mpd) or a file after the page loaded, often from inside an iframe. This opens the page with the network watched
// from before the navigation and returns what looks like media: the address, status, type, size, the frame that asked, the redirect chain and
// a kind (hls, dash, progressive, segment, ws). Read-only; the only interaction is the one fixed play script (interaction "play").
package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultNetworkWaitMs = 8000
	maxNetworkWaitMs     = 30000
	maxNetworkEntries    = 500
)

// The kinds of a media request.
const (
	mediaHLS         = "hls"
	mediaDASH        = "dash"
	mediaProgressive = "progressive"
	mediaSegment     = "segment"
	mediaWebSocket   = "ws"
)

// NetworkEntry is one media-looking request.
type NetworkEntry struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Status       int               `json:"status,omitempty"`
	ContentType  string            `json:"contentType,omitempty"`
	Size         int64             `json:"size,omitempty"`
	FrameID      string            `json:"frameId,omitempty"`
	Initiator    string            `json:"initiator,omitempty"`
	ResourceType string            `json:"resourceType,omitempty"`
	Redirects    []string          `json:"redirectChain,omitempty"`
	Kind         string            `json:"kind"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// NetworkResult is the answer of /capture-network.
type NetworkResult struct {
	URL       string         `json:"url"`
	FinalURL  string         `json:"finalUrl"`
	Entries   []NetworkEntry `json:"entries"`
	Requests  int            `json:"requestsSeen"`
	Truncated bool           `json:"truncated"`
	Played    bool           `json:"played"`
	Popups    int            `json:"popupsClosed"`
}

type captureNetworkRequest struct {
	URL         string `json:"url"`
	WaitMs      int    `json:"waitMs,omitempty"`
	Interaction string `json:"interaction,omitempty"`
	RequestID   string `json:"requestId,omitempty"`
}

// classifyMedia says what kind of media request this is ("" = not media). The address (without its query), the response type and Chrome's own
// resource type all count; an HTML or JSON answer is never media whatever its address looks like.
func classifyMedia(rawURL, mime, resourceType string) string {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	p := ""
	if u, err := url.Parse(rawURL); err == nil {
		p = strings.ToLower(path.Ext(u.Path))
	}
	switch {
	case strings.Contains(m, "mpegurl") || p == ".m3u8" || p == ".m3u":
		return mediaHLS
	case m == "application/dash+xml" || p == ".mpd":
		return mediaDASH
	case m == "text/html" || m == "application/json" || m == "text/javascript" || m == "application/javascript" || m == "text/css" || strings.HasPrefix(m, "image/") || strings.HasPrefix(m, "font/"):
		return ""
	case m == "video/mp2t" || p == ".ts" || p == ".m4s" || p == ".aac" || p == ".cmfv" || p == ".cmfa":
		return mediaSegment
	case strings.HasPrefix(m, "video/") || strings.HasPrefix(m, "audio/") || m == "application/octet-stream" && isMediaExt(p):
		return mediaProgressive
	case isMediaExt(p):
		return mediaProgressive
	case resourceType == string(proto.NetworkResourceTypeMedia):
		return mediaProgressive
	}
	return ""
}

func isMediaExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mkv", ".m4v", ".mov", ".flv", ".avi", ".mp3", ".m4a", ".ogg", ".oga", ".ogv", ".wav", ".opus":
		return true
	}
	return false
}

// keepHeader lists the response headers worth keeping as evidence of a media request (never cookies or credentials).
var keepHeader = map[string]bool{
	"content-type": true, "content-length": true, "content-range": true, "accept-ranges": true, "server": true, "via": true, "cache-control": true,
	"access-control-allow-origin": true, "x-cache": true, "cf-ray": true, "age": true, "last-modified": true,
}

// networkLog collects the requests of one page; safe for the event goroutine and the reader.
type networkLog struct {
	mu      sync.Mutex
	byID    map[proto.NetworkRequestID]*NetworkEntry
	order   []proto.NetworkRequestID
	seen    int
	ws      []NetworkEntry
	dropped bool
}

func newNetworkLog() *networkLog {
	return &networkLog{byID: map[proto.NetworkRequestID]*NetworkEntry{}}
}

func (l *networkLog) requestSent(e *proto.NetworkRequestWillBeSent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur, ok := l.byID[e.RequestID]
	if !ok {
		l.seen++
		cur = &NetworkEntry{}
		l.byID[e.RequestID] = cur
		l.order = append(l.order, e.RequestID)
	} else if e.RedirectResponse != nil {
		cur.Redirects = append(cur.Redirects, cur.URL)
	}
	cur.URL, cur.Method, cur.ResourceType, cur.FrameID = e.Request.URL, e.Request.Method, string(e.Type), string(e.FrameID)
	if e.Initiator != nil {
		cur.Initiator = e.Initiator.URL
		if cur.Initiator == "" {
			cur.Initiator = string(e.Initiator.Type)
		}
	}
}

func (l *networkLog) responseReceived(e *proto.NetworkResponseReceived) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur, ok := l.byID[e.RequestID]
	if !ok || e.Response == nil {
		return
	}
	cur.Status, cur.ContentType = e.Response.Status, e.Response.MIMEType
	hdr := map[string]string{}
	for name, v := range e.Response.Headers {
		if ln := strings.ToLower(name); keepHeader[ln] {
			hdr[ln] = v.String()
		}
	}
	if len(hdr) > 0 {
		cur.Headers = hdr
	}
}

func (l *networkLog) finished(e *proto.NetworkLoadingFinished) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cur, ok := l.byID[e.RequestID]; ok {
		cur.Size = int64(e.EncodedDataLength)
	}
}

func (l *networkLog) webSocket(e *proto.NetworkWebSocketCreated) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen++
	if len(l.ws) < maxNetworkEntries {
		l.ws = append(l.ws, NetworkEntry{URL: e.URL, Method: "GET", Kind: mediaWebSocket, ResourceType: "WebSocket", Initiator: string(e.RequestID)})
	}
}

// entries returns the media-looking requests in the order they were made, within the cap.
func (l *networkLog) entries() (out []NetworkEntry, truncated bool, seen int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out = []NetworkEntry{}
	for _, id := range l.order {
		e := *l.byID[id]
		if k := classifyMedia(e.URL, e.ContentType, e.ResourceType); k != "" {
			if len(out) >= maxNetworkEntries {
				truncated = true
				break
			}
			e.Kind = k
			out = append(out, e)
		}
	}
	for _, w := range l.ws {
		if len(out) >= maxNetworkEntries {
			truncated = true
			break
		}
		out = append(out, w)
	}
	return out, truncated, l.seen
}

// playScript is the ONE interaction this tool ever does: it starts a player, nothing else. It looks for a video element and a play button in this
// document and in the same-origin frames, calls play() / clicks the first visible play control once, and reports what it did. It types nothing,
// submits nothing and follows no link. Fixed text: never built from a page, a request or an AI.
const playScript = `(function(){
  var did = false;
  function tryDoc(d, depth){
    if (!d || depth > 3) return;
    var vids = d.querySelectorAll('video');
    for (var i = 0; i < vids.length; i++) { try { vids[i].muted = true; var p = vids[i].play(); if (p && p.catch) p.catch(function(){}); did = true; } catch (e) {} }
    var sel = ['.vjs-big-play-button','.jw-icon-display','.plyr__control--overlaid','button[aria-label*="play" i]','[class*="play-button" i]','[class*="playbutton" i]','[id*="play" i][role="button"]','.ytp-large-play-button'];
    for (var s = 0; s < sel.length && !did; s++) {
      var els = d.querySelectorAll(sel[s]);
      for (var j = 0; j < els.length; j++) { var r = els[j].getBoundingClientRect(); if (r.width > 0 && r.height > 0) { try { els[j].click(); did = true; } catch (e) {} break; } }
    }
    var fr = d.querySelectorAll('iframe');
    for (var k = 0; k < fr.length; k++) { try { tryDoc(fr[k].contentDocument, depth + 1); } catch (e) {} }
  }
  tryDoc(document, 0);
  return did;
})()`

// CaptureNetworkHandler handles POST /capture-network.
func CaptureNetworkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req captureNetworkRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required (and only url, waitMs, interaction and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if req.WaitMs < 0 {
		http.Error(w, "waitMs must not be negative", http.StatusBadRequest)
		return
	}
	if req.Interaction != "" && req.Interaction != "none" && req.Interaction != "play" {
		http.Error(w, `interaction must be "none" or "play"`, http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := runEvidence(r.Context(), req.RequestID, func(ctx context.Context) (any, error) {
		return captureNetwork(ctx, req)
	})
	writeEvidenceResult(w, req.RequestID, res, err, "capture network")
}

func captureNetwork(ctx context.Context, req captureNetworkRequest) (NetworkResult, error) {
	log := newNetworkLog()
	ep, err := openEvidencePage(ctx, req.URL, req.RequestID, log.requestSent, log.responseReceived, log.finished, log.webSocket)
	if err != nil {
		return NetworkResult{}, err
	}
	defer ep.Close()
	return captureNetworkOn(ctx, ep, log, req)
}

// captureNetworkOn waits (and plays) on an open page whose events feed log, and builds the result.
func captureNetworkOn(ctx context.Context, ep *evidencePage, log *networkLog, req captureNetworkRequest) (NetworkResult, error) {
	wait := time.Duration(clampInt(req.WaitMs, defaultNetworkWaitMs, maxNetworkWaitMs)) * time.Millisecond
	played := false
	popups := 0
	if req.Interaction == "play" {
		sleepStep := 500 * time.Millisecond
		if err := sleepCtx(ctx, sleepStep); err != nil {
			return NetworkResult{}, err
		}
		if raw, err := rodEvalExpression(ep.P, playScript); err == nil {
			_ = json.Unmarshal(raw, &played)
		}
		wait -= sleepStep
	}
	if wait > 0 {
		if err := sleepCtx(ctx, wait); err != nil {
			return NetworkResult{}, err
		}
	}
	if req.Interaction == "play" {
		popups = closeOwnPopups(ep)
	}
	entries, truncated, seen := log.entries()
	final := req.URL
	if info, err := ep.P.Info(); err == nil && info != nil {
		final = info.URL
	}
	return NetworkResult{URL: req.URL, FinalURL: final, Entries: entries, Requests: seen, Truncated: truncated, Played: played, Popups: popups}, nil
}

// closeOwnPopups closes the tabs THIS page opened (a click that opens a popup or an ad tab): only targets whose opener is this page, never another
// crawl's tab. Returns how many it closed.
func closeOwnPopups(ep *evidencePage) int {
	res, err := proto.TargetGetTargets{}.Call(ep.page)
	if err != nil {
		return 0
	}
	n := 0
	for _, t := range res.TargetInfos {
		if t.Type == "page" && t.OpenerID == ep.page.TargetID {
			if _, err := (proto.TargetCloseTarget{TargetID: t.TargetID}).Call(ep.page); err == nil {
				n++
			}
		}
	}
	return n
}
