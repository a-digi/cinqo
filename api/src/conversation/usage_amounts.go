package conversation

import (
	"math"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// usageAmounts turns the usage of one model response into what the repositories add to a run (plan/ai/conversation/step-178, step 2):
// the counts as the provider reported them, and the cost, which providers report in US dollars, in whole micro-dollars (rounded to the
// nearest one), so the sums stored in the database never drift. A response without a cost has none: it is left out of the sum and an
// all-unknown total stays unknown (NULL), which is not the same as free (0).
func usageAmounts(u chatcompleter.Usage) conversation_entity.UsageAmounts {
	a := conversation_entity.UsageAmounts{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheCreationTokens,
		ReasoningTokens:  u.ReasoningTokens,
	}
	if u.Cost != nil {
		micro := int64(math.MaxInt64)
		if f := math.Round(*u.Cost * 1e6); f < 9e18 {
			micro = int64(f)
		}
		a.CostMicroUSD = &micro
	}
	return a
}

// usageAdder is what a run's repository offers for adding one response's usage (the turn and the sub-agent repositories both do).
type usageAdder interface {
	AddUsage(id string, u conversation_entity.UsageAmounts) error
}

// usageReporter is the reportUsage callback of a run (plan/ai/conversation/step-178, step 2): each model response's whole usage goes to
// the run's row at once, so the totals are readable live. Best effort, like every progress write: a failed write never fails the turn.
func usageReporter(runs usageAdder, runID string) func(chatcompleter.Usage) {
	return func(u chatcompleter.Usage) { _ = runs.AddUsage(runID, usageAmounts(u)) }
}
