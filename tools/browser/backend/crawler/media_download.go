// media_download.go (Namako step 1, decision Q4: full downloads allowed) — POST /fetch-media: download a media address to the captures folder,
// streamed to disk with hard caps. This is the one route that writes media bytes, so it has its own scope (tool:browser:download), its own
// switch (security settings: allowMediaDownloads, default off), and is never an AI tool.
//
//   - progressive: the file itself.
//   - hls: the playlist(s), the key and init segment when named, and every segment, exactly as served (no transcoding, nothing assembled).
//     A master playlist is followed to its best variant.
//   - dash: the manifest only (its segments are not downloaded yet; the answer says so).
//
// Every connection (the first and every redirect) goes through a dialer that resolves the name itself and refuses private, link-local and
// unspecified addresses (loopback only for an allowed test host), then dials that exact address, so a name that changes its answer between the
// check and the connection cannot reach the inside. Nothing is executed, rendered or decoded.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"browser-tool-backend/shared"
)

const (
	defaultMediaBytes    = int64(512 << 20)
	hardMediaBytes       = int64(4 << 30)
	defaultMediaSeconds  = 600
	hardMediaSeconds     = 1800
	defaultMediaSegments = 5000
	hardMediaSegments    = 20000
	maxPlaylistBytes     = 2 << 20
	maxKeyBytes          = 64 << 10
	maxMediaRedirects    = 5
)

type fetchMediaRequest struct {
	URL         string `json:"url"`
	Kind        string `json:"kind"`
	MaxBytes    int64  `json:"maxBytes,omitempty"`
	MaxSeconds  int    `json:"maxSeconds,omitempty"`
	MaxSegments int    `json:"maxSegments,omitempty"`
	Referer     string `json:"referer,omitempty"`
	RequestID   string `json:"requestId,omitempty"`
}

// MediaResult is the answer of /fetch-media.
type MediaResult struct {
	CaptureID          string     `json:"captureId"`
	Kind               string     `json:"kind"`
	Files              []artifact `json:"files"`
	TotalBytes         int64      `json:"totalBytes"`
	DurationHint       float64    `json:"durationHint,omitempty"`
	SegmentsTotal      int        `json:"segmentsTotal,omitempty"`
	SegmentsDownloaded int        `json:"segmentsDownloaded,omitempty"`
	Truncated          bool       `json:"truncated"`
	Encrypted          bool       `json:"encrypted,omitempty"`
	Note               string     `json:"note,omitempty"`
}

// ---- the safe client ----

// resolveForMedia resolves host and returns the addresses a media request may use; an empty result is an error.
func resolveForMedia(ctx context.Context, host string) ([]net.IP, error) {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("could not resolve host %q: %w", host, err)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	var ok []net.IP
	for _, ip := range ips {
		if isDisallowedIP(ip) && !testHostMayBeLoopback(host, ip) {
			return nil, fmt.Errorf("refusing %q: it resolves to a private/internal address", host)
		}
		ok = append(ok, ip)
	}
	if len(ok) == 0 {
		return nil, fmt.Errorf("could not resolve host %q", host)
	}
	return ok, nil
}

// safeMediaClient is the HTTP client of every media request: dial-time address check, no proxy, no cookies, bounded redirects.
func safeMediaClient(userAgent string) *http.Client {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolveForMedia(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range ips {
				c, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if derr == nil {
					return c, nil
				}
				last = derr
			}
			return nil, last
		},
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		DisableCompression:    true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxMediaRedirects {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect to a non-http address refused")
			}
			return nil
		},
	}
}

func mediaUserAgent() string {
	if ua := rodCurrentUserAgent(); ua != "" {
		return ua
	}
	return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
}

// mediaGet opens one GET. The caller closes the body.
func mediaGet(ctx context.Context, client *http.Client, target, referer, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	if referer != "" {
		req.Header.Set("Referer", referer)
		if u, err := url.Parse(referer); err == nil && u.Host != "" {
			req.Header.Set("Origin", u.Scheme+"://"+u.Host)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("the server answered %d for %s", resp.StatusCode, target)
	}
	return resp, nil
}

// readLimited reads at most limit bytes and says whether more was there.
func readLimited(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

// mediaTypeRefused says whether a response type can never be a media file (a page, a script, JSON, an image).
func mediaTypeRefused(contentType string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	return strings.HasPrefix(m, "text/") || m == "application/json" || m == "application/javascript" || m == "application/xhtml+xml" ||
		strings.HasPrefix(m, "image/") || strings.HasPrefix(m, "font/")
}

// ---- HLS ----

type hlsVariant struct {
	URL       string
	Bandwidth int64
}

type hlsSegment struct {
	URL      string
	Duration float64
}

type hlsPlaylist struct {
	Master    bool
	Variants  []hlsVariant
	Segments  []hlsSegment
	MapURI    string
	KeyURI    string
	Encrypted bool // a KEY with a METHOD other than NONE
	Ended     bool
}

var hlsAttr = func(line, name string) string {
	i := strings.Index(line, name+"=")
	if i < 0 {
		return ""
	}
	rest := line[i+len(name)+1:]
	if strings.HasPrefix(rest, `"`) {
		if j := strings.Index(rest[1:], `"`); j >= 0 {
			return rest[1 : 1+j]
		}
		return ""
	}
	if j := strings.IndexByte(rest, ','); j >= 0 {
		return rest[:j]
	}
	return rest
}

// resolveHTTP resolves ref against base and returns it only if it is an http(s) address.
func resolveHTTP(base *url.URL, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > 4000 {
		return "", false
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	abs := base.ResolveReference(r)
	if (abs.Scheme != "http" && abs.Scheme != "https") || abs.Host == "" {
		return "", false
	}
	return abs.String(), true
}

// parseHLS reads a playlist (master or media). Lines it does not understand are ignored; an address that is not http(s) is dropped.
func parseHLS(text string, base *url.URL) hlsPlaylist {
	var pl hlsPlaylist
	var pendingDuration float64
	var pendingBW int64 = -1
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pl.Master = true
			bw, _ := strconv.ParseInt(hlsAttr(line, "BANDWIDTH"), 10, 64)
			pendingBW = bw
		case strings.HasPrefix(line, "#EXTINF:"):
			d := strings.TrimPrefix(line, "#EXTINF:")
			if i := strings.IndexByte(d, ','); i >= 0 {
				d = d[:i]
			}
			pendingDuration, _ = strconv.ParseFloat(strings.TrimSpace(d), 64)
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if u, ok := resolveHTTP(base, hlsAttr(line, "URI")); ok {
				pl.MapURI = u
			}
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			if m := strings.ToUpper(hlsAttr(line, "METHOD")); m != "" && m != "NONE" {
				pl.Encrypted = true
				if u, ok := resolveHTTP(base, hlsAttr(line, "URI")); ok {
					pl.KeyURI = u
				}
			}
		case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
			pl.Ended = true
		case strings.HasPrefix(line, "#"):
		default:
			u, ok := resolveHTTP(base, line)
			if !ok {
				pendingBW, pendingDuration = -1, 0
				continue
			}
			if pendingBW >= 0 {
				pl.Variants = append(pl.Variants, hlsVariant{URL: u, Bandwidth: pendingBW})
				pendingBW = -1
			} else {
				pl.Segments = append(pl.Segments, hlsSegment{URL: u, Duration: pendingDuration})
				pendingDuration = 0
			}
		}
	}
	return pl
}

// bestVariant is the highest-bandwidth variant (the first of equals).
func bestVariant(vs []hlsVariant) (hlsVariant, bool) {
	if len(vs) == 0 {
		return hlsVariant{}, false
	}
	best := vs[0]
	for _, v := range vs[1:] {
		if v.Bandwidth > best.Bandwidth {
			best = v
		}
	}
	return best, true
}

// segmentExt keeps the segment's own extension when it is a plain short one, else ".bin".
func segmentExt(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		if e := strings.ToLower(path.Ext(u.Path)); len(e) >= 2 && len(e) <= 6 && captureNamePattern.MatchString("x"+e) {
			return e
		}
	}
	return ".bin"
}

// ---- the download ----

type mediaJob struct {
	ctx       context.Context
	client    *http.Client
	ua        string
	referer   string
	dir       string
	deadline  time.Time
	maxBytes  int64
	maxSegs   int
	total     int64
	files     []artifact
	truncated bool
}

func (j *mediaJob) remaining() int64 { return j.maxBytes - j.total }

func (j *mediaJob) expired() bool { return time.Now().After(j.deadline) || j.ctx.Err() != nil }

// saveStream copies body to dir/name within the remaining byte budget and the deadline, hashing what was written. When the budget runs out the
// file keeps the bytes that fit and the job is marked truncated.
func (j *mediaJob) saveStream(name, kind string, body io.Reader) error {
	path := filepath.Join(j.dir, filepath.FromSlash(name))
	if !within(j.dir, path) {
		return errors.New("invalid file name")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	budget := j.remaining()
	n, cerr := io.Copy(f, io.LimitReader(&ctxReader{ctx: j.ctx, deadline: j.deadline, r: body}, budget+1))
	if cerr == nil && n > budget {
		// one byte over the budget was read: cut the file back to the budget and stop
		if terr := f.Truncate(budget); terr != nil {
			f.Close()
			return terr
		}
		n = budget
		j.truncated = true
	}
	if cerr2 := f.Close(); cerr == nil {
		cerr = cerr2
	}
	if cerr != nil {
		if errors.Is(cerr, errMediaDeadline) {
			j.truncated = true
		} else {
			_ = os.Remove(path)
			return cerr
		}
	}
	size, sum, herr := hashFile(path)
	if herr != nil {
		return herr
	}
	j.total += size
	j.files = append(j.files, artifact{Name: name, Kind: kind, Size: size, SHA256: sum})
	return nil
}

var errMediaDeadline = errors.New("the download time limit was reached")

// ctxReader stops a copy when the context ends or the deadline passes.
type ctxReader struct {
	ctx      context.Context
	deadline time.Time
	r        io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if c.ctx.Err() != nil {
		return 0, c.ctx.Err()
	}
	if time.Now().After(c.deadline) {
		return 0, errMediaDeadline
	}
	return c.r.Read(p)
}

// fetchText downloads a small text resource (a playlist, a manifest) under a size cap.
func (j *mediaJob) fetchText(target string, limit int64) (string, error) {
	resp, err := mediaGet(j.ctx, j.client, target, j.referer, j.ua)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, cut, err := readLimited(resp.Body, limit)
	if err != nil {
		return "", err
	}
	if cut {
		return "", fmt.Errorf("%s is larger than %d bytes", target, limit)
	}
	return string(data), nil
}

// saveText stores a small text file (counts toward the byte budget).
func (j *mediaJob) saveText(name, kind, text string) error {
	if int64(len(text)) > j.remaining() {
		j.truncated = true
		return nil
	}
	a, err := writeArtifact(j.dir, name, kind, []byte(text))
	if err != nil {
		return err
	}
	j.total += a.Size
	j.files = append(j.files, a)
	return nil
}

func (j *mediaJob) progressive(target string) error {
	resp, err := mediaGet(j.ctx, j.client, target, j.referer, j.ua)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if mediaTypeRefused(resp.Header.Get("Content-Type")) {
		return fmt.Errorf("this is not a media file (the server says %q)", resp.Header.Get("Content-Type"))
	}
	ext := segmentExt(target)
	return j.saveStream("media/media"+ext, "stream_media", resp.Body)
}

func (j *mediaJob) hls(target string) (MediaResult, error) {
	res := MediaResult{Kind: "hls"}
	base, err := url.Parse(target)
	if err != nil {
		return res, err
	}
	text, err := j.fetchText(target, maxPlaylistBytes)
	if err != nil {
		return res, err
	}
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return res, errors.New("this is not an HLS playlist")
	}
	if err := j.saveText("playlist.m3u8", "manifest", text); err != nil {
		return res, err
	}
	pl := parseHLS(text, base)
	if pl.Master {
		best, ok := bestVariant(pl.Variants)
		if !ok {
			return res, errors.New("the master playlist names no variant")
		}
		mbase, err := url.Parse(best.URL)
		if err != nil {
			return res, err
		}
		mtext, err := j.fetchText(best.URL, maxPlaylistBytes)
		if err != nil {
			return res, err
		}
		if err := j.saveText("media-playlist.m3u8", "manifest", mtext); err != nil {
			return res, err
		}
		pl = parseHLS(mtext, mbase)
	}
	res.Encrypted = pl.Encrypted
	res.SegmentsTotal = len(pl.Segments)
	if pl.KeyURI != "" {
		if keyText, err := j.fetchBytes(pl.KeyURI, maxKeyBytes); err == nil {
			a, werr := writeArtifact(j.dir, "key.bin", "key", keyText)
			if werr == nil {
				j.total += a.Size
				j.files = append(j.files, a)
			}
		}
	}
	if pl.MapURI != "" && !j.expired() {
		if resp, err := mediaGet(j.ctx, j.client, pl.MapURI, j.referer, j.ua); err == nil {
			err = j.saveStream("segments/init"+segmentExt(pl.MapURI), "segment", resp.Body)
			resp.Body.Close()
			if err != nil {
				return res, err
			}
		}
	}
	for i, seg := range pl.Segments {
		if j.truncated || j.expired() || i >= j.maxSegs {
			j.truncated = true
			break
		}
		resp, err := mediaGet(j.ctx, j.client, seg.URL, j.referer, j.ua)
		if err != nil {
			if j.ctx.Err() != nil {
				return res, j.ctx.Err()
			}
			return res, fmt.Errorf("segment %d: %w", i+1, err)
		}
		err = j.saveStream(fmt.Sprintf("segments/%05d%s", i, segmentExt(seg.URL)), "segment", resp.Body)
		resp.Body.Close()
		if err != nil {
			return res, fmt.Errorf("segment %d: %w", i+1, err)
		}
		res.SegmentsDownloaded++
		res.DurationHint += seg.Duration
	}
	if pl.Encrypted {
		res.Note = "the segments are encrypted; they are stored as served, together with the key when it could be fetched"
	}
	return res, nil
}

func (j *mediaJob) fetchBytes(target string, limit int64) ([]byte, error) {
	resp, err := mediaGet(j.ctx, j.client, target, j.referer, j.ua)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, cut, err := readLimited(resp.Body, limit)
	if err != nil || cut {
		return nil, errors.New("too large")
	}
	return data, nil
}

func (j *mediaJob) dash(target string) (MediaResult, error) {
	text, err := j.fetchText(target, maxPlaylistBytes)
	if err != nil {
		return MediaResult{Kind: "dash"}, err
	}
	if !strings.Contains(text, "<MPD") {
		return MediaResult{Kind: "dash"}, errors.New("this is not a DASH manifest")
	}
	if err := j.saveText("manifest.mpd", "manifest", text); err != nil {
		return MediaResult{Kind: "dash"}, err
	}
	return MediaResult{Kind: "dash", Note: "only the DASH manifest was stored; its segments are not downloaded yet"}, nil
}

// downloadMedia runs one job into a fresh capture folder, within the request's time limit.
func downloadMedia(ctx context.Context, req fetchMediaRequest) (MediaResult, error) {
	return downloadMediaWithin(ctx, req, time.Duration(clampInt(req.MaxSeconds, defaultMediaSeconds, hardMediaSeconds))*time.Second)
}

// downloadMediaWithin is downloadMedia with the time limit given as a duration (a test uses less than a second).
func downloadMediaWithin(ctx context.Context, req fetchMediaRequest, limit time.Duration) (MediaResult, error) {
	maxBytes := req.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMediaBytes
	}
	if maxBytes > hardMediaBytes {
		maxBytes = hardMediaBytes
	}
	id, dir, err := newCapture()
	if err != nil {
		return MediaResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = removeCaptureDir(dir)
		}
	}()
	if root, rerr := capturesRoot(); rerr == nil {
		if used, _ := measure(root); capturesQuotaBytes-used < maxBytes {
			maxBytes = capturesQuotaBytes - used
		}
	}
	job := &mediaJob{ctx: ctx, client: safeMediaClient(mediaUserAgent()), ua: mediaUserAgent(), referer: req.Referer, dir: dir,
		deadline: time.Now().Add(limit), maxBytes: maxBytes,
		maxSegs: clampInt(req.MaxSegments, defaultMediaSegments, hardMediaSegments)}
	var res MediaResult
	switch req.Kind {
	case "progressive":
		err = job.progressive(req.URL)
		res = MediaResult{Kind: "progressive"}
	case "hls":
		res, err = job.hls(req.URL)
	case "dash":
		res, err = job.dash(req.URL)
	}
	if err != nil {
		return MediaResult{}, err
	}
	res.CaptureID, res.Files, res.TotalBytes, res.Truncated = id, job.files, job.total, job.truncated
	if res.Files == nil {
		res.Files = []artifact{}
	}
	ok = true
	return res, nil
}

// FetchMediaHandler handles POST /fetch-media.
func FetchMediaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fetchMediaRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url and kind are required (and only url, kind, maxBytes, maxSeconds, maxSegments, referer and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if req.Kind != "progressive" && req.Kind != "hls" && req.Kind != "dash" {
		http.Error(w, `kind must be "progressive", "hls" or "dash"`, http.StatusBadRequest)
		return
	}
	if req.MaxBytes < 0 || req.MaxSeconds < 0 || req.MaxSegments < 0 {
		http.Error(w, "maxBytes, maxSeconds and maxSegments must not be negative", http.StatusBadRequest)
		return
	}
	if req.Referer != "" {
		if u, err := url.Parse(req.Referer); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(req.Referer) > 2000 {
			http.Error(w, "referer must be an http(s) address", http.StatusBadRequest)
			return
		}
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
	ctx, cancel := context.WithCancel(r.Context())
	registerCrawlCancel(req.RequestID, cancel)
	defer func() {
		unregisterCrawlCancel(req.RequestID)
		cancel()
	}()
	res, err := downloadMedia(ctx, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("fetch media failed: %v", err), http.StatusBadGateway)
		return
	}
	writeEvidenceJSON(w, http.StatusOK, res)
}
