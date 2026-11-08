package chatcompleter

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Readers for the numbers of a provider's "usage" object (plan/ai/conversation/step-178-provider-usage-openrouter.md). A provider may
// send a field as a number, a text of digits, null, or leave it out, and the details objects only for some models. A number we merely
// want to RECORD must never fail a completion that is otherwise good, so these readers take what is usable and report "not there" for
// everything else; they never guess and never fail.

// UsageObject decodes the "usage" member of a response body into a generic map; nil when there is none or it is not an object.
func UsageObject(body []byte) map[string]any {
	var wrapper struct {
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &wrapper) != nil || len(wrapper.Usage) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(wrapper.Usage, &m) != nil {
		return nil
	}
	return m
}

// numberAt walks the path through nested objects and reads a number (a JSON number or a text of one); ok is false when the path is
// missing, the value is null, or it is anything else.
func numberAt(m map[string]any, path ...string) (float64, bool) {
	var cur any = m
	for _, key := range path {
		obj, isObj := cur.(map[string]any)
		if !isObj {
			return 0, false
		}
		cur = obj[key]
	}
	switch v := cur.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// IntAt reads a count at the path: a whole number of 0 or more, else 0 (a fraction is cut, a negative or missing value is 0).
func IntAt(m map[string]any, path ...string) int {
	f, ok := numberAt(m, path...)
	if !ok || f < 0 || f > math.MaxInt32 {
		return 0
	}
	return int(f)
}

// CostAt reads a cost in US dollars at the path: nil when it is missing, null or not a number or negative (unknown), a pointer to the
// value otherwise (0 is a real answer: a free call).
func CostAt(m map[string]any, path ...string) *float64 {
	f, ok := numberAt(m, path...)
	if !ok || f < 0 {
		return nil
	}
	return &f
}
