// analysisFlow.ts — the steps of one CV analysis, apart from the page so every branch can be script-checked (step 155). The page
// gives it the real calls; a check gives it fakes.
//
// The AI does not print the proposal (the chat would show a JSON object): the run is recorded first, the AI hands the proposal
// over by calling suggest_cv_import with the run id, and its last message is one plain sentence. Only the page can end the run
// (the AI's turn lives in the core app), so this ends it: "ready" when the turn is over, or "failed" with the reason. See
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
import { NO_PROPOSAL_MESSAGE, failureReason } from './runStatusRules'

export type RunOutcome = { status: 'ready' } | { status: 'failed'; error: string }

export interface AnalysisSteps {
  // Starts the AI conversation and gives back its id.
  startConversation(): Promise<string>
  // Records the run ("generating") for that conversation and gives back its id.
  recordRun(conversationId: string): Promise<string>
  // Sends the instruction (which names the run id) and waits for the AI's turn to end; resolves with its last message.
  askAi(conversationId: string, runId: string): Promise<string>
  // Ends the run; resolves with the run, whose aiProposal holds what the AI handed over. Rejects when the server refuses.
  endRun(runId: string, outcome: RunOutcome): Promise<{ aiProposal: unknown }>
  // What the page shows along the way.
  onConversation(conversationId: string): void
  onRun(runId: string): void
  onReply(lastMessage: string): void
  // The run list changed (a run was recorded or ended): the page reloads the history.
  onRunsChanged(): void
}

// Runs the analysis and returns the proposal the AI handed over (as stored, still to be read with parseProposal). Throws an Error
// with the message to show; the run is ended as failed (best effort) whenever it was recorded and the analysis did not produce a
// proposal, so the history never keeps a run "analysing" that nobody is working on.
export async function analyseCv(steps: AnalysisSteps): Promise<unknown> {
  const conversationId = await steps.startConversation()
  steps.onConversation(conversationId)

  const runId = await steps.recordRun(conversationId)
  steps.onRun(runId)
  steps.onRunsChanged()

  try {
    steps.onReply(await steps.askAi(conversationId, runId))
  } catch (err) {
    await failRun(steps, runId, failureReason(err))
    throw err
  }

  try {
    const run = await steps.endRun(runId, { status: 'ready' })
    steps.onRunsChanged()
    return run.aiProposal
  } catch (endErr) {
    // The server refuses "ready" when the AI never handed a proposal over.
    await failRun(steps, runId, NO_PROPOSAL_MESSAGE)
    throw new Error(`${NO_PROPOSAL_MESSAGE} (${failureReason(endErr)})`)
  }
}

// Ends a run as failed; whether that works or not never replaces the error that is being reported.
async function failRun(steps: AnalysisSteps, runId: string, reason: string): Promise<void> {
  await steps.endRun(runId, { status: 'failed', error: reason }).catch(() => undefined)
  steps.onRunsChanged()
}
