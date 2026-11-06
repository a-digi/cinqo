package katana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
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
	"katana_crawl_jobs",
	"katana_get_job_page_candidates",
	"katana_test_job_detail_picks",
	"katana_save_job_detail_picks",
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

// urlList is a list of addresses that also accepts what a model sometimes sends instead of an array: one address, several separated by
// spaces, line breaks or ", ", or the JSON text of an array. A real run (plan 192, step 10) lost two rounds to a string where an array was
// expected; the schema below allows both.
type urlList []string

func (u *urlList) UnmarshalJSON(b []byte) error {
	var arr []string
	if json.Unmarshal(b, &arr) == nil {
		*u = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && json.Unmarshal([]byte(s), &arr) == nil {
		*u = arr
		return nil
	}
	arr = nil
	for _, part := range strings.Fields(strings.ReplaceAll(s, ", ", " ")) {
		if part = strings.Trim(part, `"'`+"`,"); part != "" {
			arr = append(arr, part)
		}
	}
	*u = arr
	return nil
}

type proposeArgs struct {
	PortalLinkID string  `json:"portalLinkId" jsonschema:"the portal link to instruct"`
	JobURLs      urlList `json:"jobUrls" jsonschema:"2 to 5 addresses of JOB POSTINGS, copied exactly from the sample urls of katana_get_page_links (from the group of job links)"`
	NotJobURLs   urlList `json:"notJobUrls,omitempty" jsonschema:"optional: addresses of links that look similar but are NOT job postings (a category, a filter)"`
	NextPageURLs urlList `json:"nextPageUrls,omitempty" jsonschema:"optional: the address(es) of the link(s) to the NEXT page(s) of the list, from nextPageCandidates or the groups"`
}

// proposeSchema is the schema inferred from proposeArgs with a string allowed in addition to an array for the three address lists.
func proposeSchema() (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[proposeArgs](nil)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"jobUrls", "notJobUrls", "nextPageUrls"} {
		prop := schema.Properties[name]
		if prop == nil {
			return nil, fmt.Errorf("katana: the inferred schema has no %s property", name)
		}
		switch {
		case len(prop.Types) > 0:
			prop.Types = append(prop.Types, "string")
		case prop.Type != "":
			prop.Types = []string{prop.Type, "string"}
			prop.Type = ""
		}
		prop.Items = nil // a string has no items; the array form is checked when the list is read
	}
	return schema, nil
}

type ruleArgs struct {
	PortalLinkID string         `json:"portalLinkId" jsonschema:"the portal link to instruct"`
	Rule         *linkrule.Rule `json:"rule,omitempty" jsonschema:"optional: the rule to use; omit it to use the one katana_propose_link_rule just returned"`
}

type jobPageArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link"`
	JobID        string `json:"jobId,omitempty" jsonschema:"optional: a saved job of this link to look at; default is its oldest saved job"`
}

type elementPick struct {
	Selector  string `json:"selector" jsonschema:"a CSS selector copied from the candidates"`
	Attribute string `json:"attribute,omitempty" jsonschema:"optional: read this attribute instead of the text (for example datetime)"`
}

type picksArgs struct {
	PortalLinkID        string       `json:"portalLinkId" jsonschema:"the portal link"`
	Title               *elementPick `json:"title,omitempty" jsonschema:"where the job title is; leave out when the page does not state it"`
	Company             *elementPick `json:"company,omitempty" jsonschema:"where the company is; leave out when the page does not state it"`
	Location            *elementPick `json:"location,omitempty" jsonschema:"where the place is; leave out when the page does not state it"`
	PostedAt            *elementPick `json:"postedAt,omitempty" jsonschema:"where the posting date is; leave out when the page does not state it"`
	DescriptionSelector string       `json:"descriptionSelector,omitempty" jsonschema:"the box that holds the whole description"`
	DescriptionExclude  []string     `json:"descriptionExclude,omitempty" jsonschema:"selectors inside the description box to leave out (a share bar, an apply button)"`
	DescriptionCutAfter []string     `json:"descriptionCutAfter,omitempty" jsonschema:"texts after which the description stops (for example Similar jobs)"`
}

// RegisterTools registers the Katana tools.
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
	proposeIn, err := proposeSchema()
	if err != nil {
		panic(err) // a programming error caught by the first test run, not a runtime condition
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "katana_propose_link_rule",
		InputSchema: proposeIn,
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
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_crawl_jobs",
		Description: "Katana engine. Crawl the jobs of this link with its saved job list rule (the same as the Crawl now button) and WAIT until the crawl " +
			"has ended: status completed (with the number of jobs), failed, or running (still going: call it again to keep waiting). Needs a saved " +
			"job list rule. When the link already has jobs it answers already_crawled at once. The job details can only be picked after this.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a stateArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "crawl-jobs", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_get_job_page_candidates",
		Description: "Katana engine, job details. Load the page of a CRAWLED job of this link and return, for the title, company, location, date and " +
			"description, the best 3 places on the page (selector, a short value, the reasons), the page's own notices (a reduced page, not the job), " +
			"the structured data it states, what inside the description box is probably furniture, and `suggested`: the picks the top candidates make. " +
			"Needs at least one crawled job; otherwise it says so (then STOP and tell the person to crawl the jobs).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a jobPageArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "job-page", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_test_job_detail_picks",
		Description: "Katana engine, job details. Run the picks on up to 3 real crawled jobs (the real extraction, nothing is saved) and return per job " +
			"the values read, the description length and its start, flags (blocking ones must be fixed) and a verdict. Omit all picks to test " +
			"`suggested` from katana_get_job_page_candidates.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a picksArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "test-picks", a)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "katana_save_job_detail_picks",
		Description: "Katana engine, job details. Save the picks as this link's job-detail instructions. Only picks whose katana_test_job_detail_picks " +
			"gave verdict ok in this session can be saved. Omit all picks to save `suggested`. After saving, the job details are done: do not redo them.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a picksArgs) (*mcp.CallToolResult, any, error) {
		return call(ctx, "save-picks", a)
	})
}
