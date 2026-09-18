// semantic_model.go lets a user download one of a small, hardcoded
// catalog of pretrained GloVe word-vector files, pick one as "active,"
// and have the deterministic "Match now" algorithm use it for a
// semantic fallback (job_match_algorithm.go, semantic_vectors.go) —
// entirely offline once downloaded, no AI conversation, no per-match
// network call. Deliberately NOT baked into the Go binary via go:embed
// (the smallest catalog entry alone is well over a hundred MB) — every
// entry is fetched into TOOL_DB_DIR/models on demand instead, keeping
// the binary itself small. See
// plan/ai/tools/career/step-XX-semantic-match-models.md.
package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// errModelDownloadInProgress/errModelNotReady are the two
// caller-mistake conditions this file's own handlers translate into
// 409 Conflict — mirrors errCrawlAlreadyRunning's own "the UI's own
// busy-state should already prevent this, not a server failure"
// reasoning (crawl_runs.go).
var (
	errModelDownloadInProgress = errors.New("a download is already in progress for this model")
	errModelNotReady           = errors.New("model has not finished downloading yet")
)

// modelDownloadHTTPClient's own generous timeout matches
// crawlNowHTTPClient's own reasoning (crawl_now.go) — the largest
// catalog entry below is over 2GB, which can legitimately take a long
// time on a slow connection.
var modelDownloadHTTPClient = &http.Client{Timeout: 60 * time.Minute}

// modelCatalogEntry is one hardcoded, known-good download option —
// there is no way for a caller to name an arbitrary URL; only these
// exact entries can ever be requested (modelDownloadHandler validates
// against this list). Approx sizes are for the Settings page's own
// description panel, not enforced against the real download.
type modelCatalogEntry struct {
	ID                string
	Label             string
	Dimensions        int
	Corpus            string
	Quality           string
	ArchiveURL        string
	ArchiveMemberPath string
	ApproxDownloadMB  int
	ApproxExtractedMB int
}

var modelCatalog = []modelCatalogEntry{
	{
		ID:                "glove-6b-50d",
		Label:             "GloVe 6B (50d)",
		Dimensions:        50,
		Corpus:            "Wikipedia + Gigaword (2014), ~400K word vocabulary",
		Quality:           "Fastest to load, roughest semantic signal — good for trying this feature out.",
		ArchiveURL:        "https://nlp.stanford.edu/data/glove.6B.zip",
		ArchiveMemberPath: "glove.6B.50d.txt",
		ApproxDownloadMB:  822,
		ApproxExtractedMB: 163,
	},
	{
		ID:                "glove-6b-100d",
		Label:             "GloVe 6B (100d)",
		Dimensions:        100,
		Corpus:            "Wikipedia + Gigaword (2014), ~400K word vocabulary",
		Quality:           "Balanced default — noticeably better than 50d, still fast to load.",
		ArchiveURL:        "https://nlp.stanford.edu/data/glove.6B.zip",
		ArchiveMemberPath: "glove.6B.100d.txt",
		ApproxDownloadMB:  822,
		ApproxExtractedMB: 347,
	},
	{
		ID:                "glove-6b-300d",
		Label:             "GloVe 6B (300d)",
		Dimensions:        300,
		Corpus:            "Wikipedia + Gigaword (2014), ~400K word vocabulary",
		Quality:           "Richest representation of this same 2014-era vocabulary.",
		ArchiveURL:        "https://nlp.stanford.edu/data/glove.6B.zip",
		ArchiveMemberPath: "glove.6B.300d.txt",
		ApproxDownloadMB:  822,
		ApproxExtractedMB: 990,
	},
	{
		ID:                "glove-42b-300d",
		Label:             "GloVe 42B (300d)",
		Dimensions:        300,
		Corpus:            "Common Crawl, ~1.9M word vocabulary",
		Quality:           "Much larger vocabulary — better coverage of niche/modern terms — but a bigger download and slower to load.",
		ArchiveURL:        "https://nlp.stanford.edu/data/glove.42B.300d.zip",
		ArchiveMemberPath: "glove.42B.300d.txt",
		ApproxDownloadMB:  1750,
		ApproxExtractedMB: 2700,
	},
	{
		ID:                "glove-840b-300d",
		Label:             "GloVe 840B (300d)",
		Dimensions:        300,
		Corpus:            "Common Crawl, ~2.2M word vocabulary",
		Quality:           "Best available coverage and quality — largest download and slowest to load.",
		ArchiveURL:        "https://nlp.stanford.edu/data/glove.840B.300d.zip",
		ArchiveMemberPath: "glove.840B.300d.txt",
		ApproxDownloadMB:  2030,
		ApproxExtractedMB: 5600,
	},
}

func findCatalogEntry(id string) *modelCatalogEntry {
	for i := range modelCatalog {
		if modelCatalog[i].ID == id {
			return &modelCatalog[i]
		}
	}
	return nil
}

// semanticModel is one semantic_models row (db.go) — nil FilePath/
// ErrorMessage/DownloadedAt exactly track the corresponding column
// being NULL, same convention crawlRun (crawl_runs.go) already
// established for this tool's other status-tracking rows.
type semanticModel struct {
	ID              string
	Status          string
	ProgressPercent int
	ErrorMessage    *string
	FilePath        *string
	DownloadedAt    *string
	Active          bool
}

func ensureModelRow(id string) error {
	_, err := jobsDB.Exec(`INSERT OR IGNORE INTO semantic_models (id) VALUES (?)`, id)
	return err
}

// startModelDownloadRow creates the row if this is the first attempt
// for id, then atomically flips it into 'downloading' — the WHERE
// clause is the real concurrency guard (mirrors startCrawlRun's own
// partial-unique-index guard, just expressed as a status filter here
// instead, since two different models may legitimately download
// concurrently and only a per-row check makes sense).
func startModelDownloadRow(id string) error {
	if err := ensureModelRow(id); err != nil {
		return err
	}
	result, err := jobsDB.Exec(
		`UPDATE semantic_models SET status = 'downloading', progress_percent = 0, error_message = NULL
		 WHERE id = ? AND status NOT IN ('downloading', 'extracting')`,
		id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errModelDownloadInProgress
	}
	return nil
}

func setModelProgress(id, status string, percent int) error {
	_, err := jobsDB.Exec(`UPDATE semantic_models SET status = ?, progress_percent = ? WHERE id = ?`, status, percent, id)
	return err
}

func markModelReady(id, filePath string) error {
	downloadedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := jobsDB.Exec(
		`UPDATE semantic_models SET status = 'ready', progress_percent = 100, file_path = ?, downloaded_at = ?, error_message = NULL WHERE id = ?`,
		filePath, downloadedAt, id,
	)
	return err
}

func markModelFailed(id, errMsg string) error {
	_, err := jobsDB.Exec(`UPDATE semantic_models SET status = 'failed', error_message = ? WHERE id = ?`, errMsg, id)
	return err
}

func scanSemanticModel(row *sql.Row) (*semanticModel, error) {
	var m semanticModel
	var errorMessage, filePath, downloadedAt sql.NullString
	var active int
	if err := row.Scan(&m.ID, &m.Status, &m.ProgressPercent, &errorMessage, &filePath, &downloadedAt, &active); err != nil {
		return nil, err
	}
	if errorMessage.Valid {
		m.ErrorMessage = &errorMessage.String
	}
	if filePath.Valid {
		m.FilePath = &filePath.String
	}
	if downloadedAt.Valid {
		m.DownloadedAt = &downloadedAt.String
	}
	m.Active = active == 1
	return &m, nil
}

func getModel(id string) (*semanticModel, error) {
	row := jobsDB.QueryRow(
		`SELECT id, status, progress_percent, error_message, file_path, downloaded_at, active FROM semantic_models WHERE id = ?`,
		id,
	)
	return scanSemanticModel(row)
}

// getActiveModel returns (nil, nil) — not sql.ErrNoRows — when no
// model is active at all, since that's this feature's own normal,
// expected default state (matching semantically empty results, not an
// error, elsewhere in this tool), not something every caller should
// have to special-case against sql.ErrNoRows individually.
func getActiveModel() (*semanticModel, error) {
	row := jobsDB.QueryRow(
		`SELECT id, status, progress_percent, error_message, file_path, downloaded_at, active FROM semantic_models WHERE active = 1`,
	)
	m, err := scanSemanticModel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

// listModelStatuses returns every row that has ever been touched
// (i.e. at least one download attempted), keyed by id — a catalog
// entry with no row here has never been downloaded at all.
func listModelStatuses() (map[string]*semanticModel, error) {
	rows, err := jobsDB.Query(
		`SELECT id, status, progress_percent, error_message, file_path, downloaded_at, active FROM semantic_models`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*semanticModel)
	for rows.Next() {
		var m semanticModel
		var errorMessage, filePath, downloadedAt sql.NullString
		var active int
		if err := rows.Scan(&m.ID, &m.Status, &m.ProgressPercent, &errorMessage, &filePath, &downloadedAt, &active); err != nil {
			return nil, err
		}
		if errorMessage.Valid {
			m.ErrorMessage = &errorMessage.String
		}
		if filePath.Valid {
			m.FilePath = &filePath.String
		}
		if downloadedAt.Valid {
			m.DownloadedAt = &downloadedAt.String
		}
		m.Active = active == 1
		result[m.ID] = &m
	}
	return result, rows.Err()
}

// selectModel marks id as the one active model, clearing whichever one
// was active before — a plain two-statement transaction, not relying
// on semantic_models_one_active_idx (db.go) to reject the "clear old,
// set new" sequence, since that guard exists to catch bugs, not to be
// routinely tripped by ordinary application flow.
func selectModel(id string) error {
	m, err := getModel(id)
	if err != nil {
		return err
	}
	if m.Status != "ready" {
		return errModelNotReady
	}

	tx, err := jobsDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE semantic_models SET active = 0 WHERE active = 1`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE semantic_models SET active = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// removeModel deletes id's own cached file and row — refused while a
// download is still in flight (errModelDownloadInProgress) so a
// goroutine's own later markModelReady/markModelFailed call never
// writes back into a row the user just deleted out from under it.
// Removing the currently active model is allowed and simply leaves no
// row active — job_match_now.go's own acquireActiveVectors already
// treats "no active model" as "fall back to literal-only matching."
func removeModel(id string) error {
	m, err := getModel(id)
	if err != nil {
		return err
	}
	if m.Status == "downloading" || m.Status == "extracting" {
		return errModelDownloadInProgress
	}
	if _, err := jobsDB.Exec(`DELETE FROM semantic_models WHERE id = ?`, id); err != nil {
		return err
	}
	if m.FilePath != nil {
		_ = os.Remove(*m.FilePath)
	}
	return nil
}

// reconcileOrphanedModelDownloads runs once at backend startup — same
// "a goroutine has no PID to reattach to after a restart" reasoning as
// reconcileOrphanedCrawlRuns (crawl_runs.go). A row stuck at
// 'downloading'/'extracting' from before this process started is
// definitely orphaned; its own partial temp file (if any) is simply
// leaked on disk rather than cleaned up here — modelsDir's own
// contents are small enough in practice for a human to clear by hand
// if this ever matters, and guessing at a stale temp file's own exact
// name here would be more fragile than helpful.
func reconcileOrphanedModelDownloads() error {
	_, err := jobsDB.Exec(
		`UPDATE semantic_models SET status = 'failed', error_message = 'Interrupted by a server restart' WHERE status IN ('downloading', 'extracting')`,
	)
	return err
}

func modelsDir() (string, error) {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return "", fmt.Errorf("TOOL_DB_DIR is not set")
	}
	dir := filepath.Join(dbDir, "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// modelCatalogResponseEntry is the wire shape GET /jobs/model/catalog
// returns — one entry per modelCatalog item, merged with that item's
// own live semantic_models row (a catalog entry never downloaded gets
// the zero-value "not_downloaded" defaults below, no row lookup miss
// treated as an error).
type modelCatalogResponseEntry struct {
	ID                string  `json:"id"`
	Label             string  `json:"label"`
	Dimensions        int     `json:"dimensions"`
	Corpus            string  `json:"corpus"`
	Quality           string  `json:"quality"`
	ApproxDownloadMB  int     `json:"approxDownloadMb"`
	ApproxExtractedMB int     `json:"approxExtractedMb"`
	Status            string  `json:"status"`
	ProgressPercent   int     `json:"progressPercent"`
	ErrorMessage      *string `json:"errorMessage,omitempty"`
	Active            bool    `json:"active"`
	DownloadedAt      *string `json:"downloadedAt,omitempty"`
}

// modelCatalogHandler handles GET /jobs/model/catalog — the Settings
// page's own single source of truth, polled while any entry is
// downloading (same "poll a status endpoint" convention crawlNowActiveHandler
// and the crawl monitor already established).
func modelCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses, err := listModelStatuses()
	if err != nil {
		http.Error(w, "failed to load model status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]modelCatalogResponseEntry, 0, len(modelCatalog))
	for _, entry := range modelCatalog {
		respEntry := modelCatalogResponseEntry{
			ID:                entry.ID,
			Label:             entry.Label,
			Dimensions:        entry.Dimensions,
			Corpus:            entry.Corpus,
			Quality:           entry.Quality,
			ApproxDownloadMB:  entry.ApproxDownloadMB,
			ApproxExtractedMB: entry.ApproxExtractedMB,
			Status:            "not_downloaded",
		}
		if m := statuses[entry.ID]; m != nil {
			respEntry.Status = m.Status
			respEntry.ProgressPercent = m.ProgressPercent
			respEntry.ErrorMessage = m.ErrorMessage
			respEntry.Active = m.Active
			respEntry.DownloadedAt = m.DownloadedAt
		}
		response = append(response, respEntry)
	}
	writeJSON(w, response)
}

// modelDownloadHandler handles POST /jobs/model/download — starts the
// detached goroutine and responds immediately, same shape as
// crawlNowHandler.
func modelDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ModelID == "" {
		http.Error(w, "modelId is required", http.StatusBadRequest)
		return
	}
	entry := findCatalogEntry(body.ModelID)
	if entry == nil {
		http.Error(w, "unknown model id", http.StatusBadRequest)
		return
	}

	if err := startModelDownloadRow(body.ModelID); err != nil {
		if errors.Is(err, errModelDownloadInProgress) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "failed to start download: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go runModelDownload(context.Background(), *entry)

	w.WriteHeader(http.StatusAccepted)
}

// modelSelectHandler handles POST /jobs/model/select — synchronous
// (this is a plain DB update, not a long-running operation, so unlike
// download there's no reason to make the caller poll for it).
func modelSelectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ModelID == "" {
		http.Error(w, "modelId is required", http.StatusBadRequest)
		return
	}
	if err := selectModel(body.ModelID); err != nil {
		switch {
		case errors.Is(err, errModelNotReady):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "model has not been downloaded yet", http.StatusBadRequest)
		default:
			http.Error(w, "failed to select model: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// modelRemoveHandler handles DELETE /jobs/model — synchronous, same
// reasoning as modelSelectHandler.
func modelRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ModelID == "" {
		http.Error(w, "modelId is required", http.StatusBadRequest)
		return
	}
	if err := removeModel(body.ModelID); err != nil {
		switch {
		case errors.Is(err, errModelDownloadInProgress):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "model has not been downloaded", http.StatusNotFound)
		default:
			http.Error(w, "failed to remove model: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// progressReader wraps an io.Reader, calling onProgress every time the
// integer download percentage changes (never more often than that —
// deliberately coarse, so this doesn't turn into a write to jobsDB on
// every single TCP read). total <= 0 (no Content-Length header) means
// progress can't be computed at all; onProgress is simply never called,
// and the Settings page's own progress bar stays at its last known
// value until the phase itself changes.
type progressReader struct {
	io.Reader
	total      int64
	read       int64
	lastPct    int
	onProgress func(pct int)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.Reader.Read(b)
	p.read += int64(n)
	if p.total > 0 && p.onProgress != nil {
		pct := int(p.read * 100 / p.total)
		if pct != p.lastPct {
			p.lastPct = pct
			p.onProgress(pct)
		}
	}
	return n, err
}

// downloadToFile GETs url and writes the full response body to
// destPath, reporting integer percent progress via onProgress as it
// goes. The archive is always downloaded to disk first, never streamed
// straight into zip extraction — archive/zip needs random access to
// read a zip's own central directory (at the end of the file), which
// an HTTP response body alone can't provide.
func downloadToFile(ctx context.Context, url, destPath string, onProgress func(pct int)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := modelDownloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: unexpected status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	reader := &progressReader{Reader: resp.Body, total: resp.ContentLength, onProgress: onProgress}
	_, err = io.Copy(out, reader)
	return err
}

// extractZipMember copies exactly one named member out of a zip
// archive on disk into destPath — every other member (the other
// dimensions bundled in the same 6B archive, for example) is left
// untouched and never written anywhere.
func extractZipMember(zipPath, memberPath, destPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != memberPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("archive did not contain expected member %q", memberPath)
}

// runModelDownload is the detached goroutine body — rooted in
// context.Background() by its caller (modelDownloadHandler), same
// "survives the initiating request" reasoning as runCrawlNow. Always
// ends in a terminal markModelReady/markModelFailed call; the
// temporary zip is removed on every exit path, successful or not, so a
// failed attempt never leaks a partial multi-hundred-MB file.
func runModelDownload(ctx context.Context, entry modelCatalogEntry) {
	fail := func(err error) {
		_ = markModelFailed(entry.ID, err.Error())
	}

	dir, err := modelsDir()
	if err != nil {
		fail(err)
		return
	}

	tempZipPath := filepath.Join(dir, entry.ID+".download.zip")
	if err := downloadToFile(ctx, entry.ArchiveURL, tempZipPath, func(pct int) {
		_ = setModelProgress(entry.ID, "downloading", pct)
	}); err != nil {
		_ = os.Remove(tempZipPath)
		fail(err)
		return
	}

	if err := setModelProgress(entry.ID, "extracting", 0); err != nil {
		_ = os.Remove(tempZipPath)
		fail(err)
		return
	}

	destPath := filepath.Join(dir, entry.ID+".txt")
	if err := extractZipMember(tempZipPath, entry.ArchiveMemberPath, destPath); err != nil {
		_ = os.Remove(tempZipPath)
		_ = os.Remove(destPath)
		fail(err)
		return
	}
	_ = os.Remove(tempZipPath)

	if err := markModelReady(entry.ID, destPath); err != nil {
		fail(err)
	}
}
