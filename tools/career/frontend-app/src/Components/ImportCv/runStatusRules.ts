// Pure rules for the Import history list (step 155): what a run's status looks like and what can be done with it. A run is
// "generating" while the AI analyses the CV, "ready" once its proposal is there (every run from before step 155 is ready) and
// "failed" when the analysis did not produce one. No React, so they can be script-checked.
export type RunStatus = 'generating' | 'ready' | 'failed'

export interface RunLike {
  status: RunStatus
  error?: string
}

export type BadgeTone = 'amber' | 'red'

// The badge of a run in the list, or null for a ready run (it needs none: it has its proposal and its View button).
export function runBadge(run: RunLike): { label: string; tone: BadgeTone; title: string } | null {
  switch (run.status) {
    case 'generating':
      return { label: 'Analysing…', tone: 'amber', title: 'The AI is still analysing this CV, or the page was closed before it finished.' }
    case 'failed':
      return { label: 'Failed', tone: 'red', title: describeFailure(run.error).message }
    default:
      return null
  }
}

// Only a ready run has a proposal to view.
export function canViewRun(run: RunLike): boolean {
  return run.status === 'ready'
}

// What a person is told when a run could not be finished as ready: the server's message when there is one.
export function failureReason(err: unknown): string {
  const text = err instanceof Error ? err.message : String(err)
  return text.trim() === '' ? 'The analysis did not finish.' : text.trim()
}

// The text the page shows when the AI's turn ended without a proposal (the server refused to end the run as ready).
export const NO_PROPOSAL_MESSAGE = 'The AI finished without handing over a proposal. Start over, or process the CV again.'

// What the AI's own failures look like in the error text (the core's loop and the providers write them): a model that sends tool
// arguments that cannot be used, a provider that refuses a cut-off tool call, a turn that never ends, a run that ends without a
// proposal. All of them mean the chosen model is not up to this task, whatever the exact words; the page then says that instead of
// showing the raw text, which stays available behind "Show log" (plan 165, step 4).
const MODEL_NOT_CAPABLE_PATTERNS: RegExp[] = [
  /keeps sending invalid tool arguments/i,
  /must be a valid JSON object/i,
  /without a final reply/i,
  /without handing over a proposal/i,
  /did not hand over a proposal/i,
  /no longer generating/i,
  /cut off by the token limit/i,
  /response had no choices/i,
  /finish reason: length/i,
  /did not read the cv/i,
]

export const MODEL_NOT_CAPABLE_MESSAGE =
  'The model is not up to the task. Please choose a better model in Settings and process the CV again.'

export interface FailureDescription {
  // What a person reads first.
  message: string
  // True when the choice of model is the cause, so the page offers the Settings link.
  modelNotCapable: boolean
  // The real error text, shown behind "Show log"; empty when there is none.
  log: string
}

// The message for a failed run or analysis: the friendly one for a model that is not up to the task, the error's own text otherwise.
export function describeFailure(error: string | undefined | null): FailureDescription {
  const log = (error ?? '').trim()
  if (log === '') return { message: 'The analysis did not finish.', modelNotCapable: false, log: '' }
  if (MODEL_NOT_CAPABLE_PATTERNS.some((p) => p.test(log))) return { message: MODEL_NOT_CAPABLE_MESSAGE, modelNotCapable: true, log }
  return { message: log, modelNotCapable: false, log }
}

// The AI's reply when it could not read the file at all (the upload expired, the PDF has no text): the prompt tells it to answer
// "Could not read the CV: …" and to propose nothing. Without that a model that never saw the text called the document "not a CV"
// and skipped everything (plan 165, the real run of 2026-11-02).
const CV_UNREADABLE_REPLY = /^\s*could not read the cv/i

export const CV_UNREADABLE_MESSAGE = 'The CV file could not be read; it may have expired or has no text. Upload it again.'

// The first step asked the AI to read the CV and one of its tool calls was refused or rejected: the file did not reach it (a small model once
// wrote the file reference wrongly, got "tool unavailable" and carried on with nothing). Said as it is, not as "may have expired".
export const CV_READ_FAILED_MESSAGE = 'The AI could not read the CV: its request for the file was refused. Process the CV again.'

export function isCvUnreadableReply(reply: string): boolean {
  return CV_UNREADABLE_REPLY.test(reply)
}

// The AI wrote "Could not read the CV: …" without ever calling the tool that reads the PDF: the file is fine, the model copied the failure
// sentence the prompt prescribes and made up the reason (a 1.5B model did, 10 of 10 on a replay; plan 181). Said as it is, with the advice
// that helps (a stronger model), never "the file may have expired". The text names the phrase the page's failure notice recognises.
export const CV_NOT_READ_MESSAGE =
  'The AI did not read the CV: it answered without calling the tool that reads the PDF and made up the reason. The file is fine.'

// True when the turn's tool calls are known and none of them read the CV. Unknown calls (an older core) never say so.
export function modelSkippedTheRead(toolsCalled: string[] | undefined): boolean {
  return toolsCalled !== undefined && !toolsCalled.includes('pdf_to_markdown')
}
