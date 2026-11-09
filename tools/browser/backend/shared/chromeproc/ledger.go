package chromeproc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ledgerName         = "chrome-pids.json"
	previousLedgerName = "chrome-pids.previous.json"
	ledgerVersion      = 1
)

type ledgerFile struct {
	Version int     `json:"version"`
	Chromes []Entry `json:"chromes"`
}

// prepareLedger makes the folder, keeps a ledger of an earlier run aside (only when it lists something) and returns the path of the new one.
func prepareLedger(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("chromeproc: no ledger folder")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("chromeproc: cannot make the ledger folder: %w", err)
	}
	path := filepath.Join(dir, ledgerName)
	if old, err := ReadLedger(path); err == nil && len(old) > 0 {
		_ = os.Rename(path, filepath.Join(dir, previousLedgerName))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		// a damaged ledger is kept aside too, never read as truth and never overwritten silently
		_ = os.Rename(path, filepath.Join(dir, previousLedgerName+".damaged"))
	}
	return path, nil
}

// ReadLedger reads a ledger file. A missing file is os.ErrNotExist; a damaged one is an error and returns nothing.
func ReadLedger(path string) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ledgerFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("chromeproc: the ledger is damaged: %w", err)
	}
	if f.Version != ledgerVersion {
		return nil, fmt.Errorf("chromeproc: unknown ledger version %d", f.Version)
	}
	out := make([]Entry, 0, len(f.Chromes))
	for _, e := range f.Chromes {
		if e.PID > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// saveLedger writes the current list atomically (a temporary file, then a rename), so a crash never leaves half a file. A failure is
// not an error for the caller: the tool must never stop because it could not write its ledger.
func (r *Registry) saveLedger() {
	r.mu.Lock()
	path := r.ledgerPath
	r.mu.Unlock()
	if path == "" {
		return
	}
	raw, err := json.MarshalIndent(ledgerFile{Version: ledgerVersion, Chromes: r.Entries()}, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".chrome-pids-*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(raw)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
	}
}
