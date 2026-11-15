// fetch_manifest.go (Namako step 8) — POST /fetch-manifest: store the TEXT of a stream's manifest (an HLS playlist or a DASH manifest) and the headers of the
// answer, in a fresh capture folder, as evidence. It downloads no media: only one small text file, read through the same safe client as the media routes
// (no private addresses, bounded redirects), so it needs no "allow media downloads" switch. Read-only, nothing is submitted.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	maxManifestBytes   = 512 << 10
	manifestTimeoutSec = 30
)

type fetchManifestRequest struct {
	URL       string `json:"url"`
	Referer   string `json:"referer,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// ManifestResult is the answer of /fetch-manifest.
type ManifestResult struct {
	CaptureID  string     `json:"captureId"`
	URL        string     `json:"url"`
	FinalURL   string     `json:"finalUrl"`
	Status     int        `json:"status"`
	CapturedAt string     `json:"capturedAt"`
	Kind       string     `json:"kind"` // hls or dash
	Artifacts  []artifact `json:"artifacts"`
}

// headerAllow lists the response headers kept as evidence. Cookies and anything that could carry a credential are never kept.
var headerAllow = map[string]bool{
	"content-type": true, "content-length": true, "content-encoding": true, "date": true, "last-modified": true, "etag": true, "server": true, "cache-control": true,
	"via": true, "age": true, "x-cache": true, "cf-ray": true, "cf-cache-status": true, "x-served-by": true, "access-control-allow-origin": true, "location": true,
}

// keptHeaders returns the allowed headers of a response, lower-cased names, one value each (the first).
func keptHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for name, values := range h {
		l := strings.ToLower(name)
		if headerAllow[l] && len(values) > 0 {
			v := strings.TrimSpace(values[0])
			if len(v) > 300 {
				v = v[:300]
			}
			out[l] = v
		}
	}
	return out
}

// manifestKind says what a text is: "hls" (an HLS playlist), "dash" (a DASH manifest) or "" (neither).
func manifestKind(text string) string {
	t := strings.TrimSpace(strings.TrimPrefix(text, "\xef\xbb\xbf"))
	switch {
	case strings.HasPrefix(t, "#EXTM3U"):
		return "hls"
	case strings.Contains(t, "<MPD"):
		return "dash"
	}
	return ""
}

// FetchManifestHandler handles POST /fetch-manifest.
func FetchManifestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fetchManifestRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required (and only url, referer and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if req.Referer != "" {
		if u, err := url.Parse(req.Referer); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(req.Referer) > 2000 {
			http.Error(w, "referer must be an http(s) address", http.StatusBadRequest)
			return
		}
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	registerCrawlCancel(req.RequestID, cancel)
	defer func() {
		unregisterCrawlCancel(req.RequestID)
		cancel()
	}()
	res, err := fetchManifest(ctx, req)
	if err != nil {
		var nm *notManifestError
		if errors.As(err, &nm) {
			http.Error(w, nm.Error(), http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, fmt.Sprintf("fetch manifest failed: %v", err), http.StatusBadGateway)
		return
	}
	writeEvidenceJSON(w, http.StatusOK, res)
}

type notManifestError struct{ url string }

func (e *notManifestError) Error() string {
	return "the address did not answer with an HLS playlist or a DASH manifest"
}

func fetchManifest(parent context.Context, req fetchManifestRequest) (ManifestResult, error) {
	ctx, cancel := context.WithTimeout(parent, manifestTimeoutSec*time.Second)
	defer cancel()
	id, dir, err := newCapture()
	if err != nil {
		return ManifestResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = removeCaptureDir(dir)
		}
	}()
	ua := mediaUserAgent()
	client := safeMediaClient(ua)
	resp, err := mediaGet(ctx, client, req.URL, req.Referer, ua)
	if err != nil {
		return ManifestResult{}, err
	}
	defer resp.Body.Close()
	data, cut, err := readLimited(resp.Body, maxManifestBytes)
	if err != nil {
		return ManifestResult{}, err
	}
	if cut {
		return ManifestResult{}, fmt.Errorf("the manifest is larger than %d bytes", maxManifestBytes)
	}
	kind := manifestKind(string(data))
	if kind == "" {
		return ManifestResult{}, &notManifestError{url: req.URL}
	}
	name := "manifest.m3u8"
	if kind == "dash" {
		name = "manifest.mpd"
	}
	res := ManifestResult{CaptureID: id, URL: req.URL, FinalURL: req.URL, Status: resp.StatusCode, CapturedAt: time.Now().UTC().Format(time.RFC3339), Kind: kind, Artifacts: []artifact{}}
	if resp.Request != nil && resp.Request.URL != nil {
		res.FinalURL = resp.Request.URL.String()
	}
	a, err := writeArtifact(dir, name, "manifest", data)
	if err != nil {
		return ManifestResult{}, err
	}
	res.Artifacts = append(res.Artifacts, a)
	h := keptHeaders(resp.Header)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, map[string]string{"name": k, "value": h[k]})
	}
	hdr, _ := json.MarshalIndent(map[string]any{"url": req.URL, "finalUrl": res.FinalURL, "status": resp.StatusCode, "capturedAt": res.CapturedAt, "headers": ordered}, "", "  ")
	ha, err := writeArtifact(dir, "headers.json", "headers", hdr)
	if err != nil {
		return ManifestResult{}, err
	}
	res.Artifacts = append(res.Artifacts, ha)
	ok = true
	return res, nil
}
