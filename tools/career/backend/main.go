// Command career-tool-backend is cinqo's career tool — it holds the
// user's own career profile (skills, experience, what they're looking
// for) and a database of crawled job postings. Requires the browser
// tool to be installed and enabled (enforced by the host, see
// plan/ai/tools/step-14-tool-dependencies.md) — but this backend never
// calls browser's own backend directly: the two cooperate only
// through the AI's own multi-tool-call orchestration in one
// conversation (browser crawls, this tool stores what was found). See
// plan/ai/tools/career/career.md and
// plan/ai/tools/career/step-01-manifest-and-package-skeleton.md.
//
// Unlike browser, this tool holds no persistent external resource
// (no shared browser session) across separate calls — every --mcp
// invocation and every HTTP-mode request opens what it needs (steps
// 2/3/4's own two SQLite databases) fresh, does the work, and is
// done. No TOOL_OWN_PORT adapter, no shared session mutex.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		runMCPServer()
		return
	}
	runHTTPServer()
}

func runHTTPServer() {
	port := os.Getenv("PORT")

	if err := initDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/profile", profileHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/jobs", jobsHandler)

	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		os.Exit(1)
	}
}

// runMCPServer speaks MCP over stdin/stdout for exactly one spawn ->
// exchange -> exit cycle, matching every other tool's own --mcp
// contract. Opens both databases too — unlike browser's own --mcp
// adapter (which never touches its shared session directly and
// relays through TOOL_OWN_PORT instead), this tool has no HTTP-mode
// sibling to relay through at all, so its own --mcp subprocess
// reads/writes career.db and jobs.db directly, the same as HTTP mode
// does.
func runMCPServer() {
	if err := initDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "career", Version: "0.1.0"}, nil)

	registerGetCareerProfile(server)
	registerUpdateCareerProfile(server)
	registerAddCareerSkill(server)
	registerRemoveCareerSkill(server)
	registerAddCareerExperience(server)
	registerRemoveCareerExperience(server)
	registerUpdateCareerExperience(server)
	registerSaveJob(server)
	registerListJobs(server)
	registerSearchJobs(server)
	registerDeleteJob(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
