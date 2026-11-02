// analysisFlow.ts — the steps of one CV analysis, apart from the page so every branch can be script-checked (step 155). The page
// gives it the real calls; a check gives it fakes.
//
// The AI does not print the proposal (the chat would show a JSON object): the run is recorded first, the AI hands the proposal
// over by calling suggest_cv_import with the run id, and its last message is one plain sentence. Only the page can end the run
// (the AI's turn lives in the core app), so this ends it: "ready" when the turn is over, or "failed" with the reason. See
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
import { buildContinuePrompt } from './buildImportPrompt'
import { CV_UNREADABLE_MESSAGE, NO_PROPOSAL_MESSAGE, failureReason, isCvUnreadableReply } from './runStatusRules'

export type RunOutcome = { status: 'ready' } | { status: 'failed'; error: string }

export interface AnalysisSteps {
  // Starts the AI conversation and gives back its id.
  startConversation(): Promise<string>
  // Records the run ("generating") for that conversation and gives back its id.
  recordRun(conversationId: string): Promise<string>
  // Sends the instruction (which names the run id), or the given follow-up message when the AI has to continue, and waits for the AI's
  // turn to end; resolves with its last message.
  askAi(conversationId: string, runId: string, followUp?: string): Promise<string>
  // Ends the run; resolves with the run, whose aiProposal holds what the AI handed over. Rejects when the server refuses.
  endRun(runId: string, outcome: RunOutcome): Promise<{ aiProposal: unknown }>
  // What the page shows along the way.
  onConversation(conversationId: string): void
  onRun(runId: string): void
  onReply(lastMessage: string): void
  // The run list changed (a run was recorded or ended): the page reloads the history.
  onRunsChanged(): void
}

// The runs this page's own code is finishing right now (a module-level set: it lives as long as the page's script, and a reload
// empties it). The upload page starts an analysis and then navigates to the run's page, whose resume must not finish the same run a
// second time.
const activeRuns = new Set<string>()

export function isAnalysisActive(runId: string): boolean {
  return activeRuns.has(runId)
}

// The part of an analysis that follows an AI turn: ends the run as ready, or asks the AI to continue a limited number of times, or
// fails the run. Shared by a fresh analysis and by a resumed one.
type SettleSteps = Pick<AnalysisSteps, 'askAi' | 'endRun' | 'onReply' | 'onRunsChanged'>

async function settleRun(steps: SettleSteps, conversationId: string, runId: string, firstReply: string): Promise<unknown> {
  let reply = firstReply
  for (let continuation = 0; ; continuation++) {
    // The AI could not read the file: whatever it handed over is not a result of this CV.
    if (isCvUnreadableReply(reply)) {
      await failRun(steps, runId, `${CV_UNREADABLE_MESSAGE} (${reply.trim()})`)
      throw new Error(CV_UNREADABLE_MESSAGE)
    }
    try {
      const run = await steps.endRun(runId, { status: 'ready' })
      steps.onRunsChanged()
      return run.aiProposal
    } catch (endErr) {
      // The server refuses "ready" when the AI never handed a proposal over (it finishes a complete draft itself). A weak model
      // often stops too early: the same conversation is asked to continue, a limited number of times, before the run fails.
      if (isNoProposalRefusal(endErr) && continuation < MAX_CONTINUATIONS) {
        reply = await ask(steps, conversationId, runId, buildContinuePrompt(runId, failureReason(endErr)))
        continue
      }
      await failRun(steps, runId, NO_PROPOSAL_MESSAGE)
      throw new Error(`${NO_PROPOSAL_MESSAGE} (${failureReason(endErr)})`)
    }
  }
}

// Runs the analysis and returns the proposal the AI handed over (as stored, still to be read with parseProposal). Throws an Error
// with the message to show; the run is ended as failed (best effort) whenever it was recorded and the analysis did not produce a
// proposal, so the history never keeps a run "analysing" that nobody is working on.
export async function analyseCv(steps: AnalysisSteps): Promise<unknown> {
  const conversationId = await steps.startConversation()
  steps.onConversation(conversationId)

  const runId = await steps.recordRun(conversationId)
  activeRuns.add(runId)
  try {
    steps.onRun(runId)
    steps.onRunsChanged()
    const reply = await ask(steps, conversationId, runId, undefined)
    return await settleRun(steps, conversationId, runId, reply)
  } finally {
    activeRuns.delete(runId)
  }
}

// What the resume needs besides the shared steps: the state of the run's AI turn and a wait for it to end (both are reads of the
// conversation, they start no AI call).
export type TurnState = 'running' | 'completed' | 'failed' | 'cancelled'

export interface ResumeSteps extends Pick<AnalysisSteps, 'askAi' | 'endRun' | 'onReply' | 'onRunsChanged'> {
  turnState(conversationId: string): Promise<TurnState | null>
  // Waits for the turn to end and resolves with its last message; rejects with the turn's own error when it failed.
  awaitTurn(conversationId: string): Promise<string>
}

export const RESUME_NO_TURN_MESSAGE = 'The analysis never started (the page was closed before the AI was asked). Process the CV again.'
export const RESUME_CANCELLED_MESSAGE = 'The analysis was stopped before it finished. Process the CV again.'

// Picks up a run that is still "generating" when its page is opened (a reload, a new tab): waits for the AI's turn to end and then
// finishes the run exactly as a fresh analysis does. Returns 'skipped' when this page's own code is already finishing the run.
export async function resumeAnalysis(
  steps: ResumeSteps,
  conversationId: string,
  runId: string,
): Promise<'skipped' | { aiProposal: unknown }> {
  if (activeRuns.has(runId)) return 'skipped'
  activeRuns.add(runId)
  try {
    const state = await steps.turnState(conversationId)
    if (state === null) {
      await failRun(steps, runId, RESUME_NO_TURN_MESSAGE)
      throw new Error(RESUME_NO_TURN_MESSAGE)
    }
    if (state === 'cancelled') {
      await failRun(steps, runId, RESUME_CANCELLED_MESSAGE)
      throw new Error(RESUME_CANCELLED_MESSAGE)
    }
    let reply: string
    try {
      reply = await steps.awaitTurn(conversationId)
      steps.onReply(reply)
    } catch (err) {
      await failRun(steps, runId, failureReason(err))
      throw err
    }
    return { aiProposal: await settleRun(steps, conversationId, runId, reply) }
  } finally {
    activeRuns.delete(runId)
  }
}

// How many times the AI is asked to continue when its turn ended without a proposal.
export const MAX_CONTINUATIONS = 2

function isNoProposalRefusal(err: unknown): boolean {
  return failureReason(err).includes('did not hand over a proposal')
}

// One AI turn; a failure of the turn itself ends the run as failed with the real text.
async function ask(steps: SettleSteps, conversationId: string, runId: string, followUp: string | undefined): Promise<string> {
  try {
    const reply = await steps.askAi(conversationId, runId, followUp)
    steps.onReply(reply)
    return reply
  } catch (err) {
    await failRun(steps, runId, failureReason(err))
    throw err
  }
}

// Ends a run as failed; whether that works or not never replaces the error that is being reported.
async function failRun(steps: Pick<AnalysisSteps, 'endRun' | 'onRunsChanged'>, runId: string, reason: string): Promise<void> {
  await steps.endRun(runId, { status: 'failed', error: reason }).catch(() => undefined)
  steps.onRunsChanged()
}
