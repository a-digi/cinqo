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
  // The instruction as a sequence of short messages (for a model that cannot follow the long one), or absent to send the long one.
  // Each message is sent when the previous turn has ended. Given the run id the messages name.
  stepMessages?(runId: string): string[]
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

async function settleRun(steps: SettleSteps, conversationId: string, runId: string, first: TurnResult): Promise<unknown> {
  let reply = first.reply
  let lastTurnError = first.turnError
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
        const next = await ask(steps, conversationId, runId, buildContinuePrompt(runId, failureReason(endErr)))
        reply = next.reply
        lastTurnError = next.turnError ?? lastTurnError
        continue
      }
      // The AI's own failure (a provider error that was retried) is kept in the log text.
      const cause = lastTurnError ? `${failureReason(endErr)}; last AI error: ${lastTurnError}` : failureReason(endErr)
      await failRun(steps, runId, `${NO_PROPOSAL_MESSAGE} (${cause})`)
      throw new Error(`${NO_PROPOSAL_MESSAGE} (${cause})`)
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
    const messages = steps.stepMessages?.(runId)
    const first = messages ? await askInSteps(steps, conversationId, runId, messages) : await ask(steps, conversationId, runId, undefined)
    return await settleRun(steps, conversationId, runId, first)
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
    let first: TurnResult
    try {
      first = { reply: await steps.awaitTurn(conversationId) }
      steps.onReply(first.reply)
    } catch (err) {
      // A provider error is treated like an early stop (the draft is on the run): the AI is asked to continue.
      if (!isRetryableTurnError(err)) {
        await failRun(steps, runId, failureReason(err))
        throw err
      }
      first = { reply: '', turnError: failureReason(err) }
    }
    return { aiProposal: await settleRun(steps, conversationId, runId, first) }
  } finally {
    activeRuns.delete(runId)
  }
}

// How many times the AI is asked to continue when its turn ended without a proposal.
export const MAX_CONTINUATIONS = 2

function isNoProposalRefusal(err: unknown): boolean {
  return failureReason(err).includes('did not hand over a proposal')
}

// What one AI turn gave: its last message, or (for a provider error that is worth another try) the error text and no message.
interface TurnResult {
  reply: string
  turnError?: string
}

// A failure of the AI's turn that the same conversation can recover from, because the draft is kept on the run: the provider cut
// the answer off at its token limit, or returned no choice at all. Other failures (invalid arguments, no tool calls...) end the run.
const RETRYABLE_TURN_ERRORS: RegExp[] = [/cut off by the token limit/i, /finish reason: length/i, /response had no choices/i]

export function isRetryableTurnError(err: unknown): boolean {
  const text = failureReason(err)
  return RETRYABLE_TURN_ERRORS.some((p) => p.test(text))
}

// One AI turn; a failure of the turn itself ends the run as failed with the real text, except a retryable provider error, which
// comes back as an empty reply so the run is asked to continue (limited by MAX_CONTINUATIONS).
async function ask(steps: SettleSteps, conversationId: string, runId: string, followUp: string | undefined): Promise<TurnResult> {
  try {
    const reply = await steps.askAi(conversationId, runId, followUp)
    steps.onReply(reply)
    return { reply }
  } catch (err) {
    if (isRetryableTurnError(err)) return { reply: '', turnError: failureReason(err) }
    await failRun(steps, runId, failureReason(err))
    throw err
  }
}

// How many steps in a row may fail before the sequence stops: one bad step (a small model sending a wrong argument) is not the end
// of the import, but a provider that is down fails every step and there is no point in sending the rest.
export const MAX_FAILED_STEPS_IN_A_ROW = 2

// One step after the first: a failure of the turn, of any kind, does not end the run here (the draft lives on the run, and whatever
// the step did not hand over is found by the end-of-run settling, which asks the same conversation to continue).
async function askStep(
  steps: SettleSteps,
  conversationId: string,
  runId: string,
  message: string,
): Promise<{ reply: string; error?: string }> {
  try {
    const reply = await steps.askAi(conversationId, runId, message)
    steps.onReply(reply)
    return { reply }
  } catch (err) {
    return { reply: '', error: failureReason(err) }
  }
}

// The instruction as short messages, one turn each, in the same conversation. The first step reads the file: it stops the sequence
// when the file could not be read (the reply says so) or when its turn failed (without the text there is nothing to go on). A later
// step that fails is skipped and the next one is sent (found by a real run: a small model sent a wrong argument in one step and the
// whole import failed with half of it stored); after MAX_FAILED_STEPS_IN_A_ROW failures the sequence stops. What was not handed
// over is found by settleRun, and the problems are kept in the result so a failed run says what went wrong. Resolves with the result
// of the last turn that worked.
async function askInSteps(steps: SettleSteps, conversationId: string, runId: string, messages: string[]): Promise<TurnResult> {
  let last: TurnResult = { reply: '' }
  let problem: string | undefined
  let failedInARow = 0
  for (let i = 0; i < messages.length; i++) {
    if (i === 0) {
      last = await ask(steps, conversationId, runId, messages[i])
      if (last.turnError !== undefined) return last
      if (isCvUnreadableReply(last.reply)) return last
      continue
    }
    const step = await askStep(steps, conversationId, runId, messages[i])
    if (step.error !== undefined) {
      problem = step.error
      failedInARow++
      if (failedInARow >= MAX_FAILED_STEPS_IN_A_ROW) break
      continue
    }
    failedInARow = 0
    last = { reply: step.reply }
  }
  return problem === undefined ? last : { ...last, turnError: problem }
}

// Ends a run as failed; whether that works or not never replaces the error that is being reported.
async function failRun(steps: Pick<AnalysisSteps, 'endRun' | 'onRunsChanged'>, runId: string, reason: string): Promise<void> {
  await steps.endRun(runId, { status: 'failed', error: reason }).catch(() => undefined)
  steps.onRunsChanged()
}
