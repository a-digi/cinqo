package conversation

import (
	"context"
	"strings"
)

// A tool call the framework itself refused (a tool that is not available) or rejected (arguments that failed the schema) is counted on the
// turn, so a caller that works in steps can tell that a step could not do what it was asked: a small model told to read a file that wrote
// the file name wrongly got "tool unavailable", apologised in one sentence and carried on with nothing (a real run, 2026-11-03).

type toolErrorKey struct{}

// withToolErrorReporter returns a context whose turn counts each refused or rejected tool call with report.
func withToolErrorReporter(ctx context.Context, report func()) context.Context {
	return context.WithValue(ctx, toolErrorKey{}, report)
}

// reportToolError counts one refused or rejected call (nothing when the turn does not count them).
func reportToolError(ctx context.Context) {
	if report, ok := ctx.Value(toolErrorKey{}).(func()); ok && report != nil {
		report()
	}
}

// refusedCall is what the framework answers a call it did not run.
const refusedCall = "tool unavailable"

// isToolFailure reports whether a tool answer means the call did not do its work for a reason of the framework: the tool was refused, or its
// arguments failed the schema. A tool that ran and reported a problem of its own is not counted.
func isToolFailure(toolResult string) bool {
	return toolResult == refusedCall || isArgumentRejection(toolResult) || strings.HasPrefix(toolResult, promotionFailedPrefix)
}
