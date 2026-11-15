// page_archive.go (Namako step 1) — POST /page-archive: one page captured as evidence. A full-page screenshot, the page's DOM as the SAFE copy the
// picker uses (no script, no iframe, no navigation), the frame tree, the media requests, and a meta file with the time (UTC), the final address and
// the capture environment — each file with its SHA-256. Stored in a capture folder (capture_store.go); the caller reads the files back with
// GET /capture-file and releases them with DELETE /capture. Read-only.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// maxScreenshotBytes bounds the PNG; a page that renders larger gets a viewport screenshot instead (and says so).
const maxScreenshotBytes = 25 << 20

type pageArchiveRequest struct {
	URL            string `json:"url"`
	IncludeNetwork bool   `json:"includeNetwork,omitempty"`
	Interaction    string `json:"interaction,omitempty"`
	WaitMs         int    `json:"waitMs,omitempty"`
	RequestID      string `json:"requestId,omitempty"`
}

// ArchiveEnv is how the capture was made.
type ArchiveEnv struct {
	Engine        string `json:"engine"`
	ChromeVersion string `json:"chromeVersion,omitempty"`
	UserAgent     string `json:"userAgent,omitempty"`
}

// ArchiveResult is the answer of /page-archive; Meta is also stored as meta.json (without its own hash, which cannot be in itself).
type ArchiveResult struct {
	CaptureID  string     `json:"captureId"`
	URL        string     `json:"url"`
	FinalURL   string     `json:"finalUrl"`
	Title      string     `json:"title,omitempty"`
	CapturedAt string     `json:"capturedAt"`
	Env        ArchiveEnv `json:"env"`
	Artifacts  []artifact `json:"artifacts"`
	Notes      []string   `json:"notes,omitempty"`
}

// PageArchiveHandler handles POST /page-archive.
func PageArchiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req pageArchiveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodySize))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required (and only url, includeNetwork, interaction, waitMs and requestId are accepted)", http.StatusBadRequest)
		return
	}
	if req.WaitMs < 0 || (req.Interaction != "" && req.Interaction != "none" && req.Interaction != "play") {
		http.Error(w, `waitMs must not be negative and interaction must be "none" or "play"`, http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := runEvidence(r.Context(), req.RequestID, func(ctx context.Context) (any, error) {
		return pageArchive(ctx, req)
	})
	writeEvidenceResult(w, req.RequestID, res, err, "page archive")
}

func pageArchive(ctx context.Context, req pageArchiveRequest) (ArchiveResult, error) {
	id, dir, err := newCapture()
	if err != nil {
		return ArchiveResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = removeCaptureDir(dir)
		}
	}()

	log := newNetworkLog()
	ep, err := openEvidencePage(ctx, req.URL, req.RequestID, log.requestSent, log.responseReceived, log.finished, log.webSocket)
	if err != nil {
		return ArchiveResult{}, err
	}
	defer ep.Close()
	capturedAt := time.Now().UTC()

	net, err := captureNetworkOn(ctx, ep, log, captureNetworkRequest{URL: req.URL, WaitMs: req.WaitMs, Interaction: req.Interaction})
	if err != nil {
		return ArchiveResult{}, err
	}
	res := ArchiveResult{CaptureID: id, URL: req.URL, FinalURL: net.FinalURL, CapturedAt: capturedAt.Format(time.RFC3339), Artifacts: []artifact{}}
	if info, ierr := ep.P.Info(); ierr == nil && info != nil {
		res.Title = info.Title
	}
	res.Env = archiveEnv()

	add := func(name, kind string, data []byte) error {
		a, err := writeArtifact(dir, name, kind, data)
		if err != nil {
			return err
		}
		res.Artifacts = append(res.Artifacts, a)
		return nil
	}

	// Screenshot of the whole page; too large, or refused by Chrome, falls back to what is visible.
	shot, serr := ep.P.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if serr != nil || len(shot) > maxScreenshotBytes {
		res.Notes = append(res.Notes, "the full-page screenshot was not possible or too large; the visible part was captured instead")
		shot, serr = ep.P.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	}
	if serr != nil {
		return ArchiveResult{}, rodCtxOr(ctx, errors.New("the screenshot failed: "+serr.Error()))
	}
	if err := add("screenshot.png", "screenshot", shot); err != nil {
		return ArchiveResult{}, err
	}

	// The page's DOM as the safe copy (no script, no iframe content as live, only http(s) addresses).
	if raw, derr := rodEvalString(ep.P, snapshotCaptureJS); derr == nil {
		if capture, perr := parseSnapshotCapture(raw); perr == nil {
			if htmlText, trunc, berr := buildSnapshotHTML(capture, res.FinalURL); berr == nil {
				if trunc {
					res.Notes = append(res.Notes, "the page copy was cut at its size limit")
				}
				if err := add("dom.html", "dom_archive", []byte(htmlText)); err != nil {
					return ArchiveResult{}, err
				}
			} else {
				res.Notes = append(res.Notes, "the page copy could not be made: "+berr.Error())
			}
		} else {
			res.Notes = append(res.Notes, "the page copy could not be read: "+perr.Error())
		}
	} else {
		res.Notes = append(res.Notes, "the page copy could not be taken: "+derr.Error())
	}

	// The frames.
	if frames, ferr := readFrames(ctx, ep, req.URL, maxFrameDepth, maxFrameCount); ferr == nil {
		data, _ := json.MarshalIndent(frames, "", "  ")
		if err := add("frames.json", "frames", data); err != nil {
			return ArchiveResult{}, err
		}
	} else {
		res.Notes = append(res.Notes, "the frames could not be read: "+ferr.Error())
	}

	if req.IncludeNetwork {
		data, _ := json.MarshalIndent(net, "", "  ")
		if err := add("network.json", "network_log", data); err != nil {
			return ArchiveResult{}, err
		}
	}

	meta, _ := json.MarshalIndent(res, "", "  ")
	if _, err := writeArtifact(dir, "meta.json", "meta", meta); err != nil {
		return ArchiveResult{}, err
	}
	ok = true
	return res, nil
}

// archiveEnv describes the engine that made the capture.
func archiveEnv() ArchiveEnv {
	env := ArchiveEnv{Engine: "rod", UserAgent: rodCurrentUserAgent()}
	if b, err := ensureRodEngine(); err == nil {
		if v, verr := rodBrowserVersion(b); verr == nil {
			env.ChromeVersion = v.Product
		}
	}
	return env
}
