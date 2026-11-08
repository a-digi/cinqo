package rod_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
	"career-tool-backend/instruction/enginetest"
	"career-tool-backend/instruction/rod"
)

func TestContract(t *testing.T) { enginetest.Run(t, rod.New()) }

func TestManifest(t *testing.T) {
	p, err := enginetest.CheckManifest("../../../manifest.json", []instruction.Engine{rod.New()})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p {
		t.Error(s)
	}
}

// The golden files are the six Rod tools exactly as the MCP tools/list returned them before the adapter existed (plan 192 step 0): names,
// descriptions and input schemas. The adapter must not change any of them.
func TestToolsAreUnchanged(t *testing.T) {
	raw, err := os.ReadFile("testdata/rod-tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	rod.New().RegisterTools(server)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	for _, tool := range res.Tools {
		b, _ := json.Marshal(tool)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		got = append(got, m)
	}
	byName := func(l []map[string]any) {
		sort.Slice(l, func(i, j int) bool { return l[i]["name"].(string) < l[j]["name"].(string) })
	}
	byName(want)
	byName(got)
	if len(got) != len(want) {
		t.Fatalf("got %d tools, want %d", len(got), len(want))
	}
	for i := range want {
		for _, key := range []string{"name", "description", "inputSchema"} {
			if !reflect.DeepEqual(got[i][key], want[i][key]) {
				t.Errorf("tool %v: %s changed", want[i]["name"], key)
			}
		}
	}
}

func TestJudgements(t *testing.T) {
	e := rod.New()
	if !e.HasListing(instruction.LinkFacts{HasAIInstructions: true}) || e.HasListing(instruction.LinkFacts{HasRule: true}) {
		t.Error("Rod counts the AI instructions as the listing, never the rule")
	}
	if p := e.JobDetailPolicy(instruction.LinkFacts{}); p.WrittenBy != instruction.WrittenByAI || !p.AutoAIRequest || p.NeedsCrawledJobs {
		t.Errorf("policy: %+v", p)
	}
}
