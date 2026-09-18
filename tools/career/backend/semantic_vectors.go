// semantic_vectors.go loads a GloVe-format word-vector file into
// memory and computes similarity between short phrases (a skill, a
// job description) — the primitive job_match_algorithm.go's own
// semantic fallback is built on. Deliberately keeps at most one
// model's vectors resident at a time, loaded on demand and unloaded
// after modelIdleUnloadAfter of inactivity, rather than kept loaded
// for the life of the process — see this feature's own design
// discussion (plan/ai/tools/career/step-XX-semantic-match-models.md):
// the biggest catalog entries are multiple GB in memory, so paying
// that cost permanently regardless of whether "Match now" is ever
// used again would be wasteful.
package main

import (
	"bufio"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// modelIdleUnloadAfter bounds how long a loaded model's vectors stay
// resident in memory after their most recent use by acquireActiveVectors
// below — var, not const, matching this codebase's own established
// convention (crawlNowSessionRetryInterval, staleCrawlRunThreshold) for
// timing values a test may need to shrink.
var modelIdleUnloadAfter = 5 * time.Minute

var (
	activeVectorsMu    sync.Mutex
	activeVectors      map[string][]float32
	activeVectorsID    string
	activeVectorsTimer *time.Timer
)

// acquireActiveVectors returns the word vectors for whichever
// semantic_models row is currently active, loading them into memory
// first if they aren't already resident, and (re)starting the
// idle-unload timer either way. Returns nil if no model is active, its
// file is missing, or loading it fails — every caller (computeJobMatch,
// via runJobMatchNow) treats a nil map as "no semantic fallback
// available," falling back to today's literal-only behavior rather than
// failing the match outright.
func acquireActiveVectors() map[string][]float32 {
	active, err := getActiveModel()
	if err != nil || active == nil || active.FilePath == nil {
		return nil
	}

	activeVectorsMu.Lock()
	defer activeVectorsMu.Unlock()

	if activeVectorsID != active.ID || activeVectors == nil {
		vectors, loadErr := loadWordVectors(*active.FilePath)
		if loadErr != nil {
			log.Printf("semantic model %s: failed to load %s: %v", active.ID, *active.FilePath, loadErr)
			return nil
		}
		activeVectors = vectors
		activeVectorsID = active.ID
	}

	if activeVectorsTimer != nil {
		activeVectorsTimer.Stop()
	}
	activeVectorsTimer = time.AfterFunc(modelIdleUnloadAfter, func() {
		activeVectorsMu.Lock()
		defer activeVectorsMu.Unlock()
		activeVectors = nil
		activeVectorsID = ""
	})

	return activeVectors
}

// loadWordVectors parses a GloVe-format text file ("word v1 v2 ... vN",
// space-separated, one word per line) into a map keyed by the literal
// word. Malformed lines (a value that fails to parse as a float) are
// skipped rather than failing the whole load — a single corrupted line
// somewhere in a multi-gigabyte file shouldn't discard every other
// word in it.
func loadWordVectors(path string) (map[string][]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vectors := make(map[string][]float32)
	scanner := bufio.NewScanner(f)
	// The default 64KB token buffer is too small for the highest-
	// dimension catalog entries (300 floats/line, comfortably over
	// 64KB once every line's own word is included) — grown generously
	// rather than tuned exactly to today's largest entry.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		vec := make([]float32, 0, len(fields)-1)
		ok := true
		for _, raw := range fields[1:] {
			v, err := strconv.ParseFloat(raw, 32)
			if err != nil {
				ok = false
				break
			}
			vec = append(vec, float32(v))
		}
		if ok {
			vectors[fields[0]] = vec
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vectors, nil
}

// wordTokenPattern-equivalent tokenization, done by hand (no regexp
// needed): lowercase letters/digits are kept as one running token,
// everything else is a separator — matches how GloVe's own vocabulary
// was built (lowercased, punctuation-stripped).
func tokenizeForVectors(text string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// phraseVector averages the vectors of every token in text found in
// vectors — a simple, standard bag-of-words sentence embedding. ok is
// false when none of the phrase's own tokens are in vocabulary at all
// (a fully unknown phrase deliberately contributes no signal, rather
// than a misleading all-zero vector that would compare as maximally
// dissimilar to everything).
func phraseVector(text string, vectors map[string][]float32) (vec []float32, ok bool) {
	var sum []float32
	var count int
	for _, tok := range tokenizeForVectors(text) {
		v, found := vectors[tok]
		if !found {
			continue
		}
		if sum == nil {
			sum = make([]float32, len(v))
		}
		if len(v) != len(sum) {
			continue
		}
		for i, x := range v {
			sum[i] += x
		}
		count++
	}
	if count == 0 {
		return nil, false
	}
	for i := range sum {
		sum[i] /= float32(count)
	}
	return sum, true
}

// cosineSimilarity is the standard dot-product-over-magnitudes
// similarity measure, in [-1, 1] for any two non-zero vectors of equal
// length — 0 for mismatched lengths or either vector being all-zero
// (unreachable via phraseVector, which never returns a zero vector,
// but guarded here anyway since this is a general-purpose primitive).
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
