package katana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
	"career-tool-backend/portal/linkrule"
)

// The Katana AI tools (plan 192, step 5). Each one is a thin client of the engine's internal service (instruction/katana/service), which
// runs in Career's long-running process, because this MCP subprocess cannot reach the Browser. The tools hold no logic of their own.
// Answers are compact JSON; no tool returns HTML or a raw link list.

const toolScope = "tool:career:portals"

var toolNames = []string{
	"katana_get_instruction_state",
	"katana_get_page_links",
	"katana_propose_link_rule",
	"katana_test_link_rule",
	"katana_save_link_rule",
}

func toolSpecs() []instruction.ToolSpec {
	out := make([]instruction.ToolSpec, len(toolNames))
	for i, n := range toolNames {
		out[i] = instruction.ToolSpec{Name: n, Scope: toolScope}
	}
	return out
}

// call posts to the service on Career's own loopback port with the tool's service token.
func call(ctx context.Context, op string, args any) (*mcp.CallToolResult, any, error) {
	port, token := os.Getenv("TOOL_OWN_PORT"), os.Getenv("TOOL_SERVICE_TOKEN")
	if port == "" || token == "" {
		return errText("the Career service is not running: open the Career app and try again"), nil, nil
	}
	body, err := json.Marshal(args)
	if err != nil {
		return errText(err.Error()), nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%s/ai/katana/%s", port, op), bytes.NewReader(body))
	if err != nil {
		return errText(err.Error()), nil, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errText("the Career service could not be reached: " + err.Error()), nil, nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errText(err.Error()), nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		var p struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &p) == nil && p.Message != "" {
			return errText(p.Message + " [" + p.Code + "]"), nil, nil
		}
		return errText(fmt.Sprintf("the Career service answered %d", resp.StatusCode)), nil, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		compact.Reset()
		compact.Write(raw)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: compact.String()}}}, nil, nil
}

func errText(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}, IsError: true}
}

type stateArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link"`
}

type pageLinksArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link to instruct"`
	URL          string `json:"url,omitempty" jsonschema:"optional: another page of the SAME site (for example page 2 of the list); default is the link's own page"`
}

type proposeArgs struct {
	PortalLinkID string   `json:"portalLinkId" jsonschema:"the portal link to instruct"`
	JobURLs      []string `json:"jobUrls" jsonschema:"2 to 5 addresses of JOB POSTINGS, copied exactly from the sample urls of katana_get_page_links (from the group of job links)"`
	NotJobURLs   []string `json:"notJobUrls,omitempty" jsonschema:"optional: addresses of links that look similar but are NOT job postings (a category, a filter)"`
	NextPageURLs []string `json:"nextPageUrls,omitempty" jsonschema:"optional: the address(es) of the link(s) to the NEXT page(s) of the list, from nextPageCandidates or the groups"`
}

type ruleArgs struct {
	PortalLinkID string         `json:"portalLinkId" jsonschema:"the portal link to instruct"`
	Rule         *linkrule.Rule `json:"rule,omitempty" jsonschema:"optional: the rule to use; omit it to use the one katana_propose_link_rule just returned"`
}

// RegisterTools registers the four job-list tools.
func registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_get_instruction_state",
		Description: "Katana engine. CALL THIS FIRST. Says where this link's crawl instructions stand: whether the job list rule is saved, how many jobs " +
			"were crawled, whether the job details are saved, whether a crawl or another AI run is working, what this session already did, and the ONE " +
			"nextStep (job_list_rule, crawl_jobs, job_details, done, wait, stop_login) with the reason. Follow nextStep: never redo a part that is saved " +
			"and never change a saved rule unless the person asked.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a stateArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "state", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_get_page_links",
		Description: "Katana engine. Load the link's job-list page and return a COMPACT digest of its links: grouped by the shape of their address " +
			"(shape, count, 3 sample addresses with their link texts), plus nextPageCandidates. The group whose samples are job postings (its last " +
			"part varies: # or *) is the job links. Never returns HTML or the raw list. Stops with a message when the site blocks the page or the " +
			"link needs a login: then STOP and tell the person.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a pageLinksArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "page-links", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_propose_link_rule",
		Description: "Katana engine. From 2 to 5 job-posting addresses you picked from the digest, work out the rule (the partial address that all job " +
			"links share) and say what it would do on the page: matchesOnPage, sampleTitles, needsConfirm, notes. Add nextPageUrls to also learn " +
			"the pagination. Nothing is saved. Next: katana_test_link_rule.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a proposeArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "propose-rule", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_test_link_rule",
		Description: "Katana engine. Run the rule on the real page (and the next one when it has pagination, at most 2 pages) and return the jobs it " +
			"finds: count, a few sample jobs, why it stopped and a verdict. Nothing is saved. Omit rule to test the last proposal.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a ruleArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "test-rule", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_save_link_rule",
		Description: "Katana engine. Save the rule as this link's job-list instructions. Only a rule whose katana_test_link_rule found jobs in this " +
			"session can be saved; otherwise it refuses. Omit rule to save the last proposal. After saving, the job list is done: do not redo it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a ruleArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "save-rule", a)
	})
}
