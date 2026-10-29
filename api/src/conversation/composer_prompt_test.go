package conversation

import (
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

func TestBuildComposerPromptListsWorkerToolsSortedAndSummarized(t *testing.T) {
	long := strings.Repeat("very long explanation ", 50)
	prompt := buildComposerPrompt([]chatcompleter.ToolDef{
		{Name: "zeta", Description: "Does Z. Then a second sentence that must not appear."},
		{Name: "alpha", Description: "First line.\nSecond line of a manual."},
		{Name: "verbose", Description: long},
		{Name: "blank", Description: "  "},
	})

	for _, want := range []string{"You are the orchestrator", "run_plan", "never do the work yourself", "Tools available to your workers:",
		"- alpha: First line.", "- zeta: Does Z.", "- blank: (no description)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	if strings.Contains(prompt, "second sentence") || strings.Contains(prompt, "Second line of a manual") {
		t.Errorf("only the first sentence/line of a description belongs in the catalog:\n%s", prompt)
	}
	if strings.Index(prompt, "- alpha:") > strings.Index(prompt, "- zeta:") {
		t.Errorf("the catalog must be sorted by name")
	}
	if strings.Contains(prompt, long) || !strings.Contains(prompt, "…") {
		t.Errorf("a verbose description must be truncated with an ellipsis")
	}
}

func TestBuildComposerPromptBoundsTheCatalog(t *testing.T) {
	tools := make([]chatcompleter.ToolDef, 0, 400)
	for i := 0; i < 400; i++ {
		tools = append(tools, chatcompleter.ToolDef{Name: strings.Repeat("t", 10) + string(rune('a'+i%26)) + strings.Repeat("x", i%7) + string(rune('a'+(i/26)%26)), Description: strings.Repeat("d", 200)})
	}
	prompt := buildComposerPrompt(tools)
	if len(prompt) > len(composerPromptIntro)+maxCatalogLength+400 {
		t.Fatalf("prompt is %d chars, the catalog is not bounded", len(prompt))
	}
	if !strings.Contains(prompt, "more tools are not listed") {
		t.Errorf("an omitted tail must be announced")
	}
}

func TestBuildComposerPromptWithoutTools(t *testing.T) {
	prompt := buildComposerPrompt(nil)
	if !strings.Contains(prompt, "(none") {
		t.Errorf("no tools must be stated plainly: %s", prompt)
	}
}

func TestWithSystemPrompt(t *testing.T) {
	in := []chatcompleter.Message{{Role: "user", Content: "hi"}}

	if got := withSystemPrompt("", in); len(got) != 1 || got[0].Role != "user" {
		t.Errorf("empty prompt must leave messages unchanged: %+v", got)
	}
	got := withSystemPrompt("be brief", in)
	if len(got) != 2 || got[0].Role != "system" || got[0].Content != "be brief" || got[1].Content != "hi" {
		t.Errorf("system prompt must come first: %+v", got)
	}
	if len(in) != 1 {
		t.Errorf("the input slice must not be modified")
	}
}

func TestWithComposerPromptOnlyForComposerConversations(t *testing.T) {
	in := []chatcompleter.Message{{Role: "user", Content: "hi"}}

	// Direct (and unknown) modes never get a prompt and never touch the DB (nil).
	for _, mode := range []string{conversation_entity.OrchestrationModeDirect, ""} {
		got, err := withComposerPrompt(nil, nil, mode, in)
		if err != nil || len(got) != 1 || got[0].Role == "system" {
			t.Errorf("mode %q: %+v, %v", mode, got, err)
		}
	}

	db := openMainDB(t)
	seedDomainTool(t, db, "list_jobs", "tool:demo:read")
	got, err := withComposerPrompt(db, []string{"tool:demo:read"}, conversation_entity.OrchestrationModeComposer, in)
	if err != nil || len(got) != 2 || got[0].Role != "system" {
		t.Fatalf("composer: %+v, %v", got, err)
	}
	if !strings.Contains(got[0].Content, "- list_jobs:") {
		t.Errorf("the composer prompt must list the tools the caller's scopes allow:\n%s", got[0].Content)
	}
	// A tool the caller has no scope for must not be advertised.
	got, _ = withComposerPrompt(db, []string{"tool:other:read"}, conversation_entity.OrchestrationModeComposer, in)
	if strings.Contains(got[0].Content, "list_jobs") {
		t.Errorf("a tool outside the caller's scopes must not appear in the catalog")
	}
}
