package instruction_test

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
)

type in struct{}

// fake is a well-formed engine; tests break single fields of it.
type fake struct {
	id       string
	tools    []instruction.ToolSpec
	register []string // tool names actually registered
	sql      string
	events   []instruction.EventSpec
	written  string
	method   string
}

func goodFake(id string) *fake {
	return &fake{
		id:       id,
		tools:    []instruction.ToolSpec{{Name: id + "_one", Scope: "career:write"}},
		register: []string{id + "_one"},
		sql:      "x = 1",
		events:   []instruction.EventSpec{{Topic: "career.portal_link.thing_done", Role: instruction.Publishes}},
		written:  instruction.WrittenByAI,
		method:   instruction.MethodInstructions,
	}
}

func (f *fake) ID() string                    { return f.id }
func (f *fake) Tools() []instruction.ToolSpec { return f.tools }
func (f *fake) RegisterTools(s *mcp.Server) {
	for _, n := range f.register {
		mcp.AddTool(s, &mcp.Tool{Name: n, Description: "d"}, func(context.Context, *mcp.CallToolRequest, in) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	}
}
func (f *fake) ListingMethod() string                 { return f.method }
func (f *fake) HasListing(instruction.LinkFacts) bool { return true }
func (f *fake) ListingReadySQL() string               { return f.sql }
func (f *fake) JobDetailPolicy(instruction.LinkFacts) instruction.JobDetailPolicy {
	return instruction.JobDetailPolicy{WrittenBy: f.written}
}
func (f *fake) Events() []instruction.EventSpec { return f.events }
