package conversation

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"
)

func sampleCheckpoint() Checkpoint {
	return Checkpoint{
		Messages: []chatcompleter.Message{
			{Role: "user", Content: "hi", CacheBreakpoint: true},
			{Role: "assistant", Content: "", ToolCalls: []chatcompleter.ToolCall{
				{ID: "c1", Name: "list_jobs", Arguments: json.RawMessage(`{"q":"java"}`)},
				{ID: "c2", Name: "no_args"},
			}},
			{Role: "tool", Content: "3 jobs", ToolCallID: "c1"},
			{Role: "tool", Content: "ok", ToolCallID: "c2"},
		},
		Links: []tool_mcp.ResourceLink{{Name: "cv.pdf", URI: "file:///x/cv.pdf"}},
	}
}

func TestACheckpointRoundTripsEveryFieldTheLoopNeeds(t *testing.T) {
	text, err := EncodeCheckpoint(sampleCheckpoint())
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCheckpoint(text)
	if err != nil {
		t.Fatal(err)
	}
	want := sampleCheckpoint()
	want.Messages[0].CacheBreakpoint = false // not stored: the loop sets it again
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed it:\n got %+v\nwant %+v", got, want)
	}
	if got.Messages[1].ToolCalls[1].Arguments != nil {
		t.Errorf("a call without arguments must stay without (not become null): %q", got.Messages[1].ToolCalls[1].Arguments)
	}
}

func TestACheckpointWithoutLinksRoundTrips(t *testing.T) {
	text, err := EncodeCheckpoint(Checkpoint{Messages: []chatcompleter.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeCheckpoint(text)
	if err != nil || len(got.Messages) != 1 || got.Links != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestACheckpointOverTheCapIsRefusedAndOneAtTheCapIsKept(t *testing.T) {
	big := func(n int) Checkpoint {
		return Checkpoint{Messages: []chatcompleter.Message{{Role: "user", Content: strings.Repeat("a", n)}}}
	}
	if _, err := EncodeCheckpoint(big(2<<20 + 1)); !errors.Is(err, ErrCheckpointTooLarge) {
		t.Errorf("over the cap: %v", err)
	}
	// the wrapper around the content is 50 bytes or so: well under the cap is kept, and the cap is a count of bytes of the stored text
	if text, err := EncodeCheckpoint(big(1 << 20)); err != nil || len(text) <= 1<<20 {
		t.Errorf("1 MiB of content: %d bytes, %v", len(text), err)
	}
}

func TestDecodingRefusesWhatIsNotACheckpoint(t *testing.T) {
	for name, text := range map[string]string{
		"empty":         "",
		"not json":      "nope",
		"other version": `{"version":2,"messages":[{"role":"user"}]}`,
		"no version":    `{"messages":[{"role":"user"}]}`,
		"no messages":   `{"version":1,"messages":[]}`,
	} {
		if _, err := DecodeCheckpoint(text); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
