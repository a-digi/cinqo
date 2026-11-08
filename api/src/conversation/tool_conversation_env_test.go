package conversation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Plan 183: core names the conversation in the environment of every AI tool call, and only core does.

func TestToolConversationEnvNamesThePlainConversationId(t *testing.T) {
	got := toolConversationEnv("0a60cef4-5079-41ee-b6ad-8d93effe1893")
	if len(got) != 1 || got[0] != "TOOL_CONVERSATION_ID=0a60cef4-5079-41ee-b6ad-8d93effe1893" {
		t.Errorf("%v", got)
	}
}

func TestToolConversationEnvPutsNothingElseIntoAProcessEnvironment(t *testing.T) {
	for _, bad := range []string{"", "a b", "a=b", "a\nTOOL_X=1", "x;rm", "é", strings.Repeat("a", 101), "../x"} {
		if got := toolConversationEnv(bad); got != nil {
			t.Errorf("%q gave %v", bad, got)
		}
	}
	if got := toolConversationEnv(strings.Repeat("a", 100)); len(got) != 1 {
		t.Errorf("a 100 character id must be accepted: %v", got)
	}
}

// invokeToolCall runs the tool process: its environment must carry the conversation id, built from invokeToolCall's own conversationID
// parameter (not from the call's arguments) and before the process is started. Checked on the source, because starting a real MCP server in a
// unit test is out of proportion; the live check runs a real tool.
func TestInvokeToolCallAddsTheConversationIdFromItsOwnParameterToTheToolEnvironment(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "chat.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == "invokeToolCall" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatal("invokeToolCall not found")
	}
	found, afterInvoke := false, false
	var invokePos token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "toolConversationEnv" {
				if len(call.Args) == 1 {
					if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "conversationID" {
						found = true
						if invokePos != token.NoPos && call.Pos() > invokePos {
							afterInvoke = true
						}
					}
				}
			}
		case *ast.SelectorExpr:
			if fun.Sel.Name == "Invoke" {
				invokePos = call.Pos()
			}
		}
		return true
	})
	if !found {
		t.Error("invokeToolCall does not add toolConversationEnv(conversationID) to the tool's environment")
	}
	if afterInvoke {
		t.Error("the conversation id is added after the tool was started")
	}
}
