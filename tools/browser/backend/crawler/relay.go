// relay.go (Namako step 14, the stream preview) — POST /relay: fetch ONE playlist, key or media part for a person who is reviewing a finding, and hand the
// bytes back in the answer. Nothing is written to disk and nothing is kept: it is a relay, not a download. It moves media bytes, so it has the scope of
// /fetch-media (tool:browser:download), its switch (the security setting "allow media downloads", default off) and is never an AI tool.
//
// What it is NOT: a general proxy. Only http(s) addresses; every connection (the first and every redirect) goes through safeMediaClient, which resolves the
// name itself and refuses private, link-local and unspecified addresses; no cookies and no authorisation of the caller are sent; one request, one answer,
// with a size cap per kind and a time limit; at most maxRelaysInFlight at once. Only three upstream facts pass back (status, content range, length): the
// host's other headers (cookies, redirects, server names) never leave the Browser.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"browser-tool-backend/shared"
)

const (
	maxRelayPartBytes  = 16 << 20
	relayTimeout       = 20 * time.Second
	maxRelaysInFlight  = 8
	maxRelayRefererLen = 2000
)

type relayRequest struct {
	URL     string `json:"url"`
	Kind    string `json:"kind"`
	Referer string `json:"referer,omitempty"`
	Range   string `json:"range,omitempty"`
}

// relaySlots bounds the relays in flight (a full house answers 429 at once instead of queueing).
var relaySlots = make(chan struct{}, maxRelaysInFlight)

// relayLimit is the most bytes one relayed answer may have, by kind.
func relayLimit(kind string) int64 {
	switch kind {
	case "playlist":
		return maxPlaylistBytes
	case "key":
		return maxKeyBytes
	default:
		return maxRelayPartBytes
	}
}

var relayRange = regexp.MustCompile(`^bytes=[0-9]{0,15}-[0-9]{0,15}$`)

// relayFetch opens the one GET (with the Range when there is one) and reads the answer within the kind's cap. An answer that is not 200/206, or that is
// bigger than the cap, is an error: a cut playlist or part would play wrongly and look like a verdict.
func relayFetch(ctx context.Context, req relayRequest) (status int, contentRange string, body []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, relayTimeout)
	defer cancel()
	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return 0, "", nil, err
	}
	ua := mediaUserAgent()
	hr.Header.Set("User-Agent", ua)
	hr.Header.Set("Accept", "*/*")
	hr.Header.Set("Accept-Encoding", "identity")
	if req.Referer != "" {
		hr.Header.Set("Referer", req.Referer)
		if u, perr := url.Parse(req.Referer); perr == nil && u.Host != "" {
			hr.Header.Set("Origin", u.Scheme+"://"+u.Host)
		}
	}
	if req.Range != "" {
		hr.Header.Set("Range", req.Range)
	}
	resp, err := safeMediaClient(ua).Do(hr)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, "", nil, fmt.Errorf("the server answered %d", resp.StatusCode)
	}
	limit := relayLimit(req.Kind)
	if resp.ContentLength > limit {
		return 0, "", nil, fmt.Errorf("the answer is larger than the %d byte limit", limit)
	}
	data, more, err := readLimited(resp.Body, limit)
	if err != nil {
		return 0, "", nil, err
	}
	if more {
		return 0, "", nil, fmt.Errorf("the answer is larger than the %d byte limit", limit)
	}
	return resp.StatusCode, resp.Header.Get("Content-Range"), data, nil
}

// RelayHandler handles POST /relay.
func RelayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req relayRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url and kind are required (and only url, kind, referer and range are accepted)", http.StatusBadRequest)
		return
	}
	if req.Kind != "playlist" && req.Kind != "part" && req.Kind != "key" {
		http.Error(w, `kind must be "playlist", "part" or "key"`, http.StatusBadRequest)
		return
	}
	if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		http.Error(w, "url must be an http(s) address", http.StatusBadRequest)
		return
	}
	if req.Referer != "" {
		if u, err := url.Parse(req.Referer); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(req.Referer) > maxRelayRefererLen {
			http.Error(w, "referer must be an http(s) address", http.StatusBadRequest)
			return
		}
	}
	if req.Range != "" && !relayRange.MatchString(req.Range) {
		http.Error(w, "range must look like bytes=0-1023", http.StatusBadRequest)
		return
	}
	if !shared.MediaDownloadsAllowed() {
		writeEvidenceJSON(w, http.StatusForbidden, map[string]string{
			"code":    "media_downloads_disabled",
			"message": "media downloads are switched off; a person must turn on \"allow media downloads\" in the Browser security settings",
		})
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	select {
	case relaySlots <- struct{}{}:
		defer func() { <-relaySlots }()
	default:
		w.Header().Set("Retry-After", "2")
		http.Error(w, "too many relays at once", http.StatusTooManyRequests)
		return
	}
	status, contentRange, body, err := relayFetch(r.Context(), req)
	if err != nil {
		http.Error(w, "relay failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Relay-Status", strconv.Itoa(status))
	if contentRange != "" {
		h.Set("Content-Range", contentRange)
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
