package crawler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeCrawlLogIsLineBasedAndReadsBack(t *testing.T) {
	entry := crawlLogEntry{
		ID:        "07692846-4035-4af0-9cb1-3c5af9b5cdb3",
		CreatedAt: "2026-10-30 19:52:12.184",
		Fields:    []crawlRequestField{{Label: "title", Selector: "h2"}, {Label: "company", Selector: ".company"}},
		Pages:     make([]crawlLogPageEntry, 2),
	}
	entry.Pages[0].URL = "https://example.test/a"
	entry.Pages[1].URL = "https://example.test/b"
	entry.Pages[1].HTML = "<html>" + strings.Repeat("x", 50000) + "</html>"

	raw, err := encodeCrawlLog(entry)
	if err != nil {
		t.Fatal(err)
	}
	if lines := bytes.Count(raw, []byte("\n")); lines < 10 {
		t.Fatalf("a crawl log must be spread over many lines for the file reader, got %d", lines)
	}
	var back crawlLogEntry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != entry.ID || len(back.Fields) != 2 || back.Pages[0].URL != "https://example.test/a" || back.Pages[1].HTML != entry.Pages[1].HTML {
		t.Fatalf("round trip lost data: %+v", back)
	}
	// a file written before the change (compact) still reads the same
	compact, _ := json.Marshal(entry)
	var old crawlLogEntry
	if err := json.Unmarshal(compact, &old); err != nil || old.ID != entry.ID {
		t.Fatalf("compact file: %v", err)
	}
}
