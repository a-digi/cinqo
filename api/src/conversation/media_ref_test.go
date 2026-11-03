package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

const (
	givenID = "9c0d24e4-c211-4b9d-9c3f-0824929159a3" // in the prompt of the real run
	slipID  = "9c0d24e4-c211-4b9d-9c3f-082492915a3"  // what Phi wrote: one character dropped
)

func urlArg(t *testing.T, call chatcompleter.ToolCall) string {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	s, _ := args["url"].(string)
	return s
}

func TestTheRealSlipOfTheCVImportIsPutRight(t *testing.T) {
	call := callOf("pdf_to_markdown", `{"url":"media:`+slipID+`"}`)
	got := correctMediaReferences(call, []string{givenID})
	if urlArg(t, got) != "media:"+givenID {
		t.Errorf("url = %q", urlArg(t, got))
	}
}

func TestOnlyAClearSlipOfAnIdOfThisConversationIsCorrected(t *testing.T) {
	other := "11111111-2222-3333-4444-555555555555"
	near := givenID[:len(givenID)-2] + "00" // two characters changed
	for name, c := range map[string]struct {
		url   string
		known []string
		want  string
	}{
		"a dropped character":           {"media:" + slipID, []string{givenID}, "media:" + givenID},
		"an added character":            {"media:" + givenID[:10] + "X" + givenID[10:], []string{givenID}, "media:" + givenID},
		"a changed character":           {"media:" + givenID[:5] + "Z" + givenID[6:], []string{givenID}, "media:" + givenID},
		"two edits are still a slip":    {"media:" + near, []string{givenID}, "media:" + givenID},
		"three edits are not":           {"media:" + givenID[:len(givenID)-3] + "000", []string{givenID}, "media:" + givenID[:len(givenID)-3] + "000"},
		"an id that is known stays":     {"media:" + givenID, []string{givenID, other}, "media:" + givenID},
		"another known id stays":        {"media:" + other, []string{givenID, other}, "media:" + other},
		"a made-up id stays":            {"media:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", []string{givenID}, "media:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
		"nothing known: nothing to fix": {"media:" + slipID, nil, "media:" + slipID},
		"not a media reference":         {"https://example.com/a.pdf", []string{givenID}, "https://example.com/a.pdf"},
		"a plain value near an id":      {slipID, []string{givenID}, slipID},
		"an empty reference":            {"media:", []string{givenID}, "media:"},
	} {
		got := urlArg(t, correctMediaReferences(callOf("pdf_to_markdown", `{"url":"`+c.url+`"}`), c.known))
		if got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestAnAmbiguousSlipIsLeftAlone(t *testing.T) {
	// Two known ids are equally near: guessing could open the wrong file.
	a := "aaaaaaaa-0000-0000-0000-000000000001"
	b := "aaaaaaaa-0000-0000-0000-000000000002"
	slip := "aaaaaaaa-0000-0000-0000-000000000003"
	if got := urlArg(t, correctMediaReferences(callOf("pdf_to_markdown", `{"url":"media:`+slip+`"}`), []string{a, b})); got != "media:"+slip {
		t.Errorf("an ambiguous slip must not be guessed: %q", got)
	}
	// The nearer of two wins when it is strictly nearer.
	near := "aaaaaaaa-0000-0000-0000-00000000000"
	if got := urlArg(t, correctMediaReferences(callOf("pdf_to_markdown", `{"url":"media:`+near+`"}`), []string{a, "zzzzzzzz-9999-9999-9999-999999999999"})); got != "media:"+a {
		t.Errorf("the one clearly nearest id wins: %q", got)
	}
}

func TestOtherArgumentsAndShapesAreNotTouched(t *testing.T) {
	for name, args := range map[string]string{
		"a number":       `{"url": 7}`,
		"a nested value": `{"x": {"url": "media:` + slipID + `"}}`,
		"not json":       `media:` + slipID,
		"no arguments":   ``,
		"a list":         `["media:` + slipID + `"]`,
		"no media in it": `{"url": "file:///a.pdf", "n": 3}`,
	} {
		call := callOf("t", args)
		got := correctMediaReferences(call, []string{givenID})
		if string(got.Arguments) != args {
			t.Errorf("%s: arguments changed to %s", name, got.Arguments)
		}
	}
	keep := correctMediaReferences(callOf("t", `{"other":"keep","url":"media:`+slipID+`","n":3}`), []string{givenID})
	var m map[string]any
	_ = json.Unmarshal(keep.Arguments, &m)
	if m["other"] != "keep" || m["n"] != float64(3) || m["url"] != "media:"+givenID {
		t.Errorf("only the reference changes: %v", m)
	}
}

func TestMediaIDsAreReadFromTheMessagesOfTheConversation(t *testing.T) {
	msgs := []chatcompleter.Message{
		{Role: "user", Content: `Call the tool with url="media:` + givenID + `" now.`},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: `again media:` + givenID + ` and media:aaaaaaaa-1111 and media:short`},
	}
	got := mediaIDsIn(msgs)
	if len(got) != 2 || got[0] != givenID || got[1] != "aaaaaaaa-1111" {
		t.Errorf("ids = %v (each once, at least 8 characters, in order)", got)
	}
}

func TestEditDistanceStopsAtTheLimit(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"abc", "abc", 0}, {"abc", "abd", 1}, {"abc", "ab", 1}, {"abc", "abcd", 1}, {"abc", "xbz", 2}, {"abc", "xyz", 3}, {"abcdef", "ab", 3}, {"", "ab", 2}, {"", "abc", 3}} {
		if got := editDistance(c.a, c.b, 2); got != c.want {
			t.Errorf("editDistance(%q, %q, 2) = %d, want %d (limit+1 = 3 means more than the limit)", c.a, c.b, got, c.want)
		}
	}
}

// Through the real loop: the tool receives the corrected reference, and the model is never told about the slip.
func TestTheLoopRunsTheToolWithTheCorrectedReference(t *testing.T) {
	var got string
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		got = urlArg(t, call)
		return "the CV text", nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
	s := &argScript{calls: []chatcompleter.ToolCall{callOf("list_personas", `{"url":"media:`+slipID+`"}`)}}
	_, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: `read url="media:` + givenID + `"`}}, offered(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "media:"+givenID {
		t.Errorf("the tool got %q", got)
	}
	if len(s.results) != 1 || s.results[0] != "the CV text" {
		t.Errorf("results = %v", s.results)
	}
}
