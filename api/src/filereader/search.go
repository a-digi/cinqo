package filereader

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Match is one place the search text occurs: the line, how many bytes into that line the match starts, and a
// short preview of the text around it.
type Match struct {
	N       int64  `json:"n"`
	Offset  int64  `json:"offset"`
	Preview string `json:"preview"`
}

// SearchResult is one call's matches. When Done is false there may be more: ask again from NextFrom and
// NextOffset (a position inside a line, because a match can be far inside a very long one). A call ends early
// (Done false) once it has found its limit of matches or used up its bytes or its time.
type SearchResult struct {
	Matches      []Match `json:"matches"`
	NextFrom     int64   `json:"nextFrom"`
	NextOffset   int64   `json:"nextOffset"`
	Done         bool    `json:"done"`
	ScannedBytes int64   `json:"scannedBytes"`
}

// Search looks for query, as plain text (never a pattern), from byte fromOffset of line `from` on, and returns
// up to limit matches (clamped to SearchMaxMatches), one per occurrence. It streams through the file in chunks
// with a bounded buffer, so it finds a match anywhere in a line, however long, and a call is bounded in bytes
// (SearchMaxBytes) and time (SearchBudget): a call that runs out answers with what it found and where to
// continue, never an error. Case is ignored unless caseSensitive. The search text may not contain a line break
// (a match never spans lines).
func (e *Engine) Search(ctx context.Context, path, query string, from, fromOffset int64, limit int, caseSensitive bool) (SearchResult, error) {
	if from < 1 || fromOffset < 0 || limit < 1 {
		return SearchResult{}, ErrBadRange
	}
	if query == "" || len(query) > e.lim.MaxQueryBytes || !utf8.ValidString(query) || strings.ContainsAny(query, "\x00\n\r") {
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

	c, status := e.cursorAt(ctx, f, ix, from)
	switch status {
	case atEnd:
		return SearchResult{Matches: []Match{}, NextFrom: from, Done: true}, nil
	case notIndexed:
		return SearchResult{}, ErrNotIndexed
	}
	if err := c.skipErr(); err != nil {
		return SearchResult{}, err
	}
	if c.line != from {
		return SearchResult{Matches: []Match{}, NextFrom: from, Done: true}, nil
	}

	// where to begin: `fromOffset` bytes into line `from`, or at the next line when the line is shorter
	line, lineStart := from, c.pos
	if fromOffset > 0 {
		lr, err := c.next(0, nil)
		if err != nil {
			return SearchResult{}, err
		}
		if !lr.ok {
			return SearchResult{Matches: []Match{}, NextFrom: from, Done: true}, nil
		}
		if fromOffset >= int64(lr.total) {
			line, lineStart, fromOffset = from+1, c.pos, 0
		}
	}
	return e.scan(ctx, f, ix.size, lineStart+fromOffset, line, lineStart, query, limit, caseSensitive)
}

// scan streams the file from byte `start` (which lies in line `line`, that began at byte `lineStart`).
func (e *Engine) scan(ctx context.Context, f *os.File, size, start, line, lineStart int64, query string, limit int, caseSensitive bool) (SearchResult, error) {
	needle := []byte(query)
	ascii := isASCII(needle)
	foldBytes := false // lower-case the buffer first (a non-ASCII text matched ignoring case)
	if !caseSensitive {
		needle = []byte(strings.ToLower(query))
		foldBytes = !ascii
	}

	// A match is decided once the buffer holds all of it and enough text after it for the preview, so the last
	// `keep` bytes of a chunk wait for the next one; `back` bytes before a position are kept for the preview.
	const back = 128
	const ahead = 512
	keep := len(needle) + ahead
	res := SearchResult{Matches: []Match{}}
	deadline := time.Now().Add(e.lim.SearchBudget)

	buf := make([]byte, 0, e.lim.SearchChunkBytes+keep+back)
	bufStart := start // the byte offset of buf[0]
	decided := start  // match starts before this offset have been decided
	track := start    // line and lineStart describe this offset
	reader := io.NewSectionReader(f, start, size-start)
	chunk := make([]byte, e.lim.SearchChunkBytes)

	// advance moves the line tracking forward to the offset `to`, counting the line breaks between.
	advance := func(to int64) {
		for track < to {
			rest := buf[track-bufStart : to-bufStart]
			i := bytes.IndexByte(rest, '\n')
			if i < 0 {
				track = to
				return
			}
			track += int64(i) + 1
			line++
			lineStart = track
		}
	}

	for eof := false; !eof; {
		if err := ctx.Err(); err != nil {
			return SearchResult{}, err
		}
		n, err := io.ReadFull(reader, chunk)
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			eof = true
		case err != nil:
			return SearchResult{}, err
		}
		buf = append(buf, chunk[:n]...)
		res.ScannedBytes += int64(n)

		end := bufStart + int64(len(buf))
		limitAbs := end - int64(keep)
		if eof {
			limitAbs = end
		}
		hay := buf
		if foldBytes {
			if lowered := bytes.ToLower(buf); len(lowered) == len(buf) {
				hay = lowered
			}
		}
		for decided < limitAbs {
			rel := int(decided - bufStart)
			var at int
			if caseSensitive || foldBytes {
				at = bytes.Index(hay[rel:], needle)
			} else {
				at = indexFoldASCII(hay[rel:], needle)
			}
			if at < 0 || bufStart+int64(rel+at) >= limitAbs {
				break
			}
			matchAbs := bufStart + int64(rel+at)
			advance(matchAbs)
			res.Matches = append(res.Matches, Match{N: line, Offset: matchAbs - lineStart, Preview: e.preview(buf, int(matchAbs-bufStart), len(needle), lineStart-bufStart, back, ahead, eof)})
			decided = matchAbs + int64(len(needle))
			if len(res.Matches) >= limit {
				advance(decided)
				res.NextFrom, res.NextOffset = line, decided-lineStart
				res.Done = decided >= size
				return res, nil
			}
		}
		if decided < limitAbs {
			decided = limitAbs
		}
		advance(decided)

		if eof {
			break
		}
		if res.ScannedBytes >= e.lim.SearchMaxBytes || time.Now().After(deadline) {
			res.NextFrom, res.NextOffset = line, decided-lineStart
			return res, nil
		}
		// keep only what a later match or preview can still need
		drop := int(decided-bufStart) - back
		if drop > 0 {
			copy(buf, buf[drop:])
			buf = buf[:len(buf)-drop]
			bufStart += int64(drop)
		}
	}
	res.NextFrom, res.NextOffset, res.Done = line, decided-lineStart, true
	return res, nil
}

// preview is the text around a match, at most PreviewRunes characters, from the line's own bytes in buf (a line
// break ends it), with "…" where the line goes on. at is the match's position in buf; lineStartRel is where its
// line begins relative to buf (negative when it began before the buffer); atEOF says buf ends at the end of the
// file, so nothing follows it.
func (e *Engine) preview(buf []byte, at, matchLen int, lineStartRel int64, back, ahead int, atEOF bool) string {
	lo, cutLeft := at-back, true
	if lineStartRel >= 0 && lo <= int(lineStartRel) {
		lo, cutLeft = int(lineStartRel), false
	}
	lo = max(lo, 0)

	hi, cutRight := at+matchLen+ahead, true
	if hi >= len(buf) {
		hi, cutRight = len(buf), !atEOF
	}
	if i := bytes.IndexByte(buf[at:hi], '\n'); i >= 0 {
		hi, cutRight = at+i, false
		if hi > lo && buf[hi-1] == '\r' {
			hi--
		}
	}

	// step onto character boundaries at both ends
	for lo < at && !utf8.RuneStart(buf[lo]) {
		lo++
		cutLeft = true
	}
	text := buf[lo:hi]
	if cutRight {
		text = trimPartialRune(text)
	}
	runes := []rune(sanitize(text))
	if len(runes) > e.lim.PreviewRunes {
		runes, cutRight = runes[:e.lim.PreviewRunes], true
	}
	out := string(runes)
	if cutLeft {
		out = "…" + out
	}
	if cutRight {
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
