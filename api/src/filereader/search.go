package filereader

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Match is one line that contains the search text, with a short preview of it.
type Match struct {
	N       int64  `json:"n"`
	Preview string `json:"preview"`
}

// SearchResult is one call's matches. When Done is false there may be more: ask again from NextFrom. A
// call ends early (Done false) once it has found its limit of matches or used up its bytes or its time.
type SearchResult struct {
	Matches      []Match `json:"matches"`
	NextFrom     int64   `json:"nextFrom"`
	Done         bool    `json:"done"`
	ScannedBytes int64   `json:"scannedBytes"`
}

// checkEvery is how many lines pass between two looks at the clock.
const checkEvery = 256

// Search looks for query, as plain text (never a pattern), in the lines from line `from` on, and returns up
// to limit matches (clamped to SearchMaxMatches). It streams through the file with the same bounded buffer as
// a window read, and a call is bounded in bytes (SearchMaxBytes) and time (SearchBudget): a call that runs
// out answers with what it found and NextFrom, never an error. Only the first MaxLineBytes of a line are
// searched, the same part a window shows. Case is ignored unless caseSensitive.
func (e *Engine) Search(ctx context.Context, path, query string, from int64, limit int, caseSensitive bool) (SearchResult, error) {
	if from < 1 || limit < 1 {
		return SearchResult{}, ErrBadRange
	}
	if query == "" || len(query) > e.lim.MaxQueryBytes || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return SearchResult{}, ErrBadQuery
	}
	if limit > e.lim.SearchMaxMatches {
		limit = e.lim.SearchMaxMatches
	}

	release, err := e.acquire(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	defer release()

	f, ix, err := e.open(path)
	if err != nil {
		return SearchResult{}, err
	}
	defer f.Close()

	ix.mu.Lock()
	offset, line, ok := ix.startOf(from, e.lim.CheckpointEvery)
	complete, total := ix.complete, ix.totalLines()
	ix.mu.Unlock()
	if !ok {
		if complete && from > total {
			return SearchResult{Matches: []Match{}, NextFrom: from, Done: true}, nil
		}
		return SearchResult{}, ErrNotIndexed
	}

	r := bufio.NewReaderSize(io.NewSectionReader(f, offset, ix.size-offset), 64<<10)
	if ok, err := skipLines(ctx, r, line, from); err != nil {
		return SearchResult{}, err
	} else if !ok {
		return SearchResult{Matches: []Match{}, NextFrom: from, Done: true}, nil
	}
	return e.scan(ctx, r, from, query, limit, caseSensitive)
}

func (e *Engine) scan(ctx context.Context, r *bufio.Reader, line int64, query string, limit int, caseSensitive bool) (SearchResult, error) {
	needle := []byte(query)
	ascii := isASCII(needle)
	if !caseSensitive {
		needle = []byte(strings.ToLower(query))
	}
	res := SearchResult{Matches: []Match{}}
	var scratch []byte
	deadline := time.Now().Add(e.lim.SearchBudget)

	for n := 0; ; n++ {
		if res.ScannedBytes >= e.lim.SearchMaxBytes || (n%checkEvery == 0 && time.Now().After(deadline)) {
			res.NextFrom = line
			return res, nil
		}
		if n%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return SearchResult{}, err
			}
		}
		raw, total, ok, err := readLine(r, e.lim.MaxLineBytes, scratch)
		if err != nil {
			return SearchResult{}, err
		}
		scratch = raw
		if !ok {
			res.NextFrom, res.Done = line, true
			return res, nil
		}
		res.ScannedBytes += int64(total) + 1

		hay, at := raw, 0
		switch {
		case caseSensitive:
			at = bytes.Index(raw, needle)
		case ascii:
			// no allocation per line: the common case (an ASCII search text) compares byte by byte
			at = indexFoldASCII(raw, needle)
		default:
			hay = bytes.ToLower(raw)
			at = bytes.Index(hay, needle)
		}
		if at >= 0 {
			res.Matches = append(res.Matches, Match{N: line, Preview: e.preview(raw, hay, at)})
			if len(res.Matches) >= limit {
				res.NextFrom = line + 1
				_, peekErr := r.Peek(1)
				res.Done = peekErr != nil
				return res, nil
			}
		}
		line++
	}
}

// preview is the part of a matching line around the match, at most PreviewRunes characters, with "…" where
// the line goes on. at is the byte position of the match in hay (the line, lower-cased when case is ignored).
func (e *Engine) preview(raw, hay []byte, at int) string {
	text := []rune(sanitize(raw))
	if len(text) <= e.lim.PreviewRunes {
		return string(text)
	}
	centre := utf8.RuneCount(hay[:at])
	start := centre - e.lim.PreviewRunes/3
	if start < 0 {
		start = 0
	}
	end := start + e.lim.PreviewRunes
	if end > len(text) {
		end = len(text)
		start = max(0, end-e.lim.PreviewRunes)
	}
	out := string(text[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// indexFoldASCII is bytes.Index ignoring ASCII case; needle must already be lower case.
func indexFoldASCII(hay, needle []byte) int {
	n := len(needle)
	first := needle[0]
	for i := 0; i+n <= len(hay); i++ {
		if lowerASCII(hay[i]) != first {
			continue
		}
		j := 1
		for j < n && lowerASCII(hay[i+j]) == needle[j] {
			j++
		}
		if j == n {
			return i
		}
	}
	return -1
}
