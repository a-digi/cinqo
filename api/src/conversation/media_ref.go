package conversation

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// A small model copies a long id badly: a real run of the CV import (2026-11-03) was told url="media:9c0d24e4-c211-4b9d-9c3f-0824929159a3"
// and called the tool with "...082492915a3", one character short. The tool answered "tool unavailable" and the rest of the import
// ran without the CV. A media reference the model wrote that is not an id of this conversation, but is a typo of exactly one that is,
// is put right before the tool runs. Only ids that already appear in the conversation's own messages are candidates, so nothing the
// conversation was not given can be reached this way, and a reference that is not clearly a slip is left as it is.

// mediaRefPattern finds the ids named in a message: media:<id>.
var mediaRefPattern = regexp.MustCompile(`media:([A-Za-z0-9_-]{8,64})`)

// maxMediaTypoDistance is how many single-character edits (a dropped, added or changed character) still count as a slip of the pen.
const maxMediaTypoDistance = 2

// mediaIDsIn is every media id named in the messages, each once, in order of appearance.
func mediaIDsIn(messages []chatcompleter.Message) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range messages {
		for _, match := range mediaRefPattern.FindAllStringSubmatch(m.Content, -1) {
			if !seen[match[1]] {
				seen[match[1]] = true
				ids = append(ids, match[1])
			}
		}
	}
	return ids
}

// correctMediaReferences puts right the media references of call's arguments (top level string values): a reference whose id is one of
// known stays; one that is within maxMediaTypoDistance edits of exactly one known id (the nearest, with no other at the same
// distance) becomes that id; anything else stays.
func correctMediaReferences(call chatcompleter.ToolCall, known []string) chatcompleter.ToolCall {
	if len(known) == 0 || len(call.Arguments) == 0 || !strings.Contains(string(call.Arguments), mediaArgumentScheme) {
		return call
	}
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return call
	}
	changed := false
	for key, raw := range args {
		value, ok := raw.(string)
		if !ok || !strings.HasPrefix(value, mediaArgumentScheme) {
			continue
		}
		id := strings.TrimPrefix(value, mediaArgumentScheme)
		if fixed, ok := nearestMediaID(id, known); ok && fixed != id {
			args[key] = mediaArgumentScheme + fixed
			changed = true
		}
	}
	if !changed {
		return call
	}
	fixed, err := json.Marshal(args)
	if err != nil {
		return call
	}
	call.Arguments = fixed
	return call
}

// nearestMediaID is the known id that id is a slip of, or id itself when it is known; ok is false when there is no clear single one.
func nearestMediaID(id string, known []string) (string, bool) {
	best, bestDistance, ties := "", maxMediaTypoDistance+1, 0
	for _, k := range known {
		if k == id {
			return id, true
		}
		d := editDistance(id, k, maxMediaTypoDistance)
		switch {
		case d < bestDistance:
			best, bestDistance, ties = k, d, 1
		case d == bestDistance:
			ties++
		}
	}
	if bestDistance > maxMediaTypoDistance || ties != 1 {
		return "", false
	}
	return best, true
}

// editDistance is the number of single-character edits (insert, delete, change) between a and b, or limit+1 when it is more than limit.
func editDistance(a, b string, limit int) int {
	if d := len(a) - len(b); d > limit || -d > limit {
		return limit + 1
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > limit {
			return limit + 1
		}
		prev = cur
	}
	if prev[len(b)] > limit {
		return limit + 1
	}
	return prev[len(b)]
}
