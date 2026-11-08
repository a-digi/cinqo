package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Step 93 (part B): a model that cannot form a valid tool call wastes its
// whole budget on rejected calls. The framework rejects such a call before
// the tool runs, with text starting `validating "arguments"` (a schema
// violation) or `invalid tool arguments:` (not JSON at all). The loop counts
// these per turn, helps the model with the tool's real argument names, and
// gives up when it keeps failing. See
// plan/ai/tools/career/step-93-tool-call-budget-and-weak-models.md.

// ErrToolArgumentsRejected fires when a turn accumulates too many rejected
// tool calls (maxArgumentRejectionsPerTurn).
var ErrToolArgumentsRejected = errors.New("conversation: the model keeps sending invalid tool arguments")

const (
	// hintAfterRejections: the Nth rejection of the SAME tool in a turn gets
	// the tool's allowed arguments appended.
	hintAfterRejections = 2
	// maxArgumentRejectionsPerTurn ends the turn: rejections across all
	// tools in one turn.
	maxArgumentRejectionsPerTurn = 4
	// maxRejectionDetailLen bounds the rejection text kept in the error.
	maxRejectionDetailLen = 300
)

// ArgumentRejectionError is ErrToolArgumentsRejected with the tool and the
// latest rejection, so the user sees what the model got wrong.
type ArgumentRejectionError struct {
	Tool   string
	Count  int
	Detail string
}

func (e *ArgumentRejectionError) Error() string {
	return fmt.Sprintf("%s: %d tool calls were rejected before running (last: %s) — %s", ErrToolArgumentsRejected.Error(), e.Count, e.Tool, e.Detail)
}

func (e *ArgumentRejectionError) Is(target error) bool { return target == ErrToolArgumentsRejected }

// isArgumentRejection recognises the framework's own rejection of a call's
// arguments, as opposed to a tool that ran and reported a problem.
func isArgumentRejection(toolResult string) bool {
	return strings.HasPrefix(toolResult, `validating "arguments"`) || strings.HasPrefix(toolResult, "invalid tool arguments:")
}

// argumentHint describes the tool's real arguments from the schema the
// model was offered: names, types where simple, and which are required.
// Built only from the schema, never from model output. Empty when the tool
// or its schema is unknown.
func argumentHint(tools []chatcompleter.ToolDef, name string) string {
	for _, t := range tools {
		if t.Name != name {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Type any `json:"type"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if json.Unmarshal(t.InputSchema, &schema) != nil || len(schema.Properties) == 0 {
			return ""
		}
		required := map[string]bool{}
		for _, r := range schema.Required {
			required[r] = true
		}
		names := make([]string, 0, len(schema.Properties))
		for n := range schema.Properties {
			names = append(names, n)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, n := range names {
			part := n
			if typ := simpleType(schema.Properties[n].Type); typ != "" {
				part += " (" + typ
				if required[n] {
					part += ", required"
				}
				part += ")"
			} else if required[n] {
				part += " (required)"
			}
			parts = append(parts, part)
		}
		return fmt.Sprintf("Allowed arguments for %s: %s. Send only these properties, with these types; the call is rejected before it runs otherwise.", name, strings.Join(parts, ", "))
	}
	return ""
}

// simpleType renders a JSON-schema "type" that is a plain string or a list
// of strings; anything more complex is left out of the hint.
func simpleType(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok && s != "null" {
				out = append(out, s)
			}
		}
		return strings.Join(out, " or ")
	}
	return ""
}

// argumentRejections tracks one turn's rejected calls.
type argumentRejections struct {
	perTool map[string]int
	total   int
}

// record notes a rejection of tool `name` and returns the text to give the
// model back: the original result, plus the hint from the second rejection
// of that same tool on.
func (r *argumentRejections) record(tools []chatcompleter.ToolDef, name, result string) string {
	if r.perTool == nil {
		r.perTool = map[string]int{}
	}
	r.perTool[name]++
	r.total++
	if r.perTool[name] >= hintAfterRejections {
		if hint := argumentHint(tools, name); hint != "" {
			return result + "\n\n" + hint
		}
	}
	return result
}

func (r *argumentRejections) exhausted() bool { return r.total >= maxArgumentRejectionsPerTurn }

func rejectionDetail(result string) string {
	text := strings.TrimSpace(result)
	if rs := []rune(text); len(rs) > maxRejectionDetailLen {
		text = string(rs[:maxRejectionDetailLen]) + "…"
	}
	return text
}
