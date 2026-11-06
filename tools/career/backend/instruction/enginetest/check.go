// Package enginetest is the contract every instruction engine must pass (plan 192). Check returns the problems it finds as plain sentences
// (so the suite can test itself against bad engines); Run fails a test with them. CheckRegistry compares engines with each other, and
// CheckManifest compares them with manifest.json. This package imports no engine and nothing of portal, crawl or jobs.
package enginetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
)

var (
	toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	topicPattern    = regexp.MustCompile(`^career\.[a-z][a-z_]*\.[a-z][a-z_]*$`)
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Run fails the test with every problem Check finds.
func Run(t *testing.T, e instruction.Engine) {
	t.Helper()
	for _, p := range Check(e) {
		t.Error(p)
	}
}

// Check runs the contract of one engine.
func Check(e instruction.Engine) (problems []string) {
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	id := e.ID()
	if !idPattern.MatchString(id) {
		add("engine id %q must be lower case letters, digits and underscores", id)
	}

	// Tools: well-formed, unique, prefixed unless legacy, and registered exactly as declared.
	declared := map[string]bool{}
	for _, tool := range e.Tools() {
		switch {
		case !toolNamePattern.MatchString(tool.Name):
			add("tool name %q is not lower case letters, digits and underscores", tool.Name)
		case declared[tool.Name]:
			add("tool %q is declared twice", tool.Name)
		case strings.TrimSpace(tool.Scope) == "":
			add("tool %q has no scope", tool.Name)
		case !tool.Legacy && !strings.HasPrefix(tool.Name, id+"_"):
			add("tool %q is new, so its name must start with %q (only a tool that existed before the engine may be marked legacy)", tool.Name, id+"_")
		}
		declared[tool.Name] = true
	}
	if registered, err := registeredTools(e); err != nil {
		add("RegisterTools could not be checked: %v", err)
	} else {
		for name := range registered {
			if !declared[name] {
				add("RegisterTools registers %q, which Tools() does not declare", name)
			}
		}
		for name := range declared {
			if !registered[name] {
				add("Tools() declares %q, which RegisterTools does not register", name)
			}
		}
	}

	if m := e.ListingMethod(); m != instruction.MethodInstructions && m != instruction.MethodLinks {
		add("ListingMethod is %q, want %q or %q", m, instruction.MethodInstructions, instruction.MethodLinks)
	}

	// Judgements: total over every combination of facts, and well-formed.
	for _, login := range []bool{false, true} {
		for _, rule := range []bool{false, true} {
			for _, ai := range []bool{false, true} {
				facts := instruction.LinkFacts{RequiresLogin: login, HasRule: rule, HasAIInstructions: ai}
				func() {
					defer func() {
						if r := recover(); r != nil {
							add("HasListing/JobDetailPolicy panicked for %+v: %v", facts, r)
						}
					}()
					e.HasListing(facts)
					if w := e.JobDetailPolicy(facts).WrittenBy; w != instruction.WrittenByAI && w != instruction.WrittenByPicker {
						add("JobDetailPolicy(%+v).WrittenBy is %q, want %q or %q", facts, w, instruction.WrittenByAI, instruction.WrittenByPicker)
					}
				}()
			}
		}
	}
	if sqlText := e.ListingReadySQL(); strings.TrimSpace(sqlText) == "" || strings.Contains(sqlText, ";") {
		add("ListingReadySQL must be one non-empty condition without a semicolon, got %q", sqlText)
	}

	// Events: well-formed, unique per topic and role.
	seen := map[string]bool{}
	for _, ev := range e.Events() {
		key := ev.Role + "|" + ev.Topic
		switch {
		case !topicPattern.MatchString(ev.Topic):
			add("event topic %q must look like career.<thing>.<what>", ev.Topic)
		case ev.Role != instruction.Publishes && ev.Role != instruction.Consumes:
			add("event %q has role %q, want %q or %q", ev.Topic, ev.Role, instruction.Publishes, instruction.Consumes)
		case ev.Role == instruction.Consumes && strings.TrimSpace(ev.PathSuffix) == "":
			add("consumed event %q has no path suffix", ev.Topic)
		case ev.Role == instruction.Publishes && ev.PathSuffix != "":
			add("published event %q must not have a path suffix", ev.Topic)
		case seen[key]:
			add("event %q (%s) is declared twice", ev.Topic, ev.Role)
		}
		seen[key] = true
	}
	return problems
}

// registeredTools registers the engine's tools on a fresh MCP server and lists what a client sees.
func registeredTools(e instruction.Engine) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := mcp.NewServer(&mcp.Implementation{Name: "enginetest", Version: "0"}, nil)
	e.RegisterTools(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "enginetest-client", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, err
	}
	defer clientSession.Close()
	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, tool := range res.Tools {
		out[tool.Name] = true
	}
	return out, nil
}

// CheckRegistry compares the engines with each other: no tool name is claimed by two engines and no two engines share an id.
func CheckRegistry(engines []instruction.Engine) (problems []string) {
	ids := map[string]bool{}
	tools := map[string]string{}
	for _, e := range engines {
		if ids[e.ID()] {
			problems = append(problems, fmt.Sprintf("two engines have the id %q", e.ID()))
		}
		ids[e.ID()] = true
		for _, tool := range e.Tools() {
			if other, dup := tools[tool.Name]; dup && other != e.ID() {
				problems = append(problems, fmt.Sprintf("tool %q is claimed by both %q and %q", tool.Name, other, e.ID()))
			}
			tools[tool.Name] = e.ID()
		}
	}
	return problems
}

// CheckManifest compares the engines with manifest.json: every engine tool is listed in mcp_tools with the same scope, and every consumed
// event is listed in event_listeners with the same path suffix. (The manifest also lists tools and events that belong to no engine.)
func CheckManifest(manifestPath string, engines []instruction.Engine) (problems []string, err error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m struct {
		MCPTools []struct {
			Name          string `json:"name"`
			RequiredScope string `json:"required_scope"`
		} `json:"mcp_tools"`
		EventListeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	scopes := map[string]string{}
	for _, t := range m.MCPTools {
		scopes[t.Name] = t.RequiredScope
	}
	listeners := map[string]string{}
	for _, l := range m.EventListeners {
		listeners[l.Topic] = l.PathSuffix
	}
	for _, e := range engines {
		for _, tool := range e.Tools() {
			if scope, ok := scopes[tool.Name]; !ok {
				problems = append(problems, fmt.Sprintf("engine %q: tool %q is not in manifest.json mcp_tools", e.ID(), tool.Name))
			} else if scope != tool.Scope {
				problems = append(problems, fmt.Sprintf("engine %q: tool %q has scope %q in the engine and %q in manifest.json", e.ID(), tool.Name, tool.Scope, scope))
			}
		}
		for _, ev := range e.Events() {
			if ev.Role != instruction.Consumes {
				continue
			}
			if suffix, ok := listeners[ev.Topic]; !ok {
				problems = append(problems, fmt.Sprintf("engine %q: consumed event %q is not in manifest.json event_listeners", e.ID(), ev.Topic))
			} else if suffix != ev.PathSuffix {
				problems = append(problems, fmt.Sprintf("engine %q: event %q has path suffix %q in the engine and %q in manifest.json", e.ID(), ev.Topic, ev.PathSuffix, suffix))
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}
