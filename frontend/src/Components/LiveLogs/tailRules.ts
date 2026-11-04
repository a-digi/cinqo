// tailRules.ts — the pure rules of following a log: what the browser holds, how fast to poll, and what to say when it fails. No React and
// no network here, so every branch can be script-checked.
import type { LiveBatch, LiveLine } from './types'

// How many lines the browser keeps; older ones drop (the status says how many).
export const MAX_HELD = 5000
// How many lines the first poll asks for (the start of the follow), and how many a later poll may carry (it may have a lot to catch up on).
export const FIRST_LINES = 200
export const CATCH_UP_LINES = 500

// A line the browser holds. `seq` counts from the start of the follow (a live log has no line numbers), and stays the same for a line's
// whole life, so it can be a stable key.
export interface HeldLine extends LiveLine {
  seq: number
}

export interface TailState {
  lines: HeldLine[]
  // The seq the next line gets.
  nextSeq: number
  // How many older lines were dropped to stay within the limit.
  dropped: number
  // How many times the log was truncated or replaced since the follow began.
  resets: number
  // The offset of the last line taken in (-1 while none): a line at or before it is a repeat and is ignored.
  lastOffset: number
}

export function newTail(): TailState {
  return { lines: [], nextSeq: 0, dropped: 0, resets: 0, lastOffset: -1 }
}

// Takes a batch in. A reset clears what is held (the log started over) and counts it; a line that is not after the last one taken in is a
// repeat (a poll answered twice) and is ignored; what is over the limit drops from the old end. Never changes its input.
export function mergeBatch(state: TailState, batch: LiveBatch, max: number = MAX_HELD): TailState {
  const base: TailState = batch.reset ? { ...newTail(), nextSeq: state.nextSeq, resets: state.resets + 1 } : state
  let last = base.lastOffset
  let next = base.nextSeq
  const added: HeldLine[] = []
  for (const line of batch.lines) {
    if (line.offset <= last) continue
    last = line.offset
    added.push({ ...line, seq: next++ })
  }
  if (added.length === 0 && !batch.reset) return state
  let lines = base.lines.length === 0 ? added : base.lines.concat(added)
  let dropped = base.dropped
  if (lines.length > max) {
    dropped += lines.length - max
    lines = lines.slice(lines.length - max)
  }
  return { lines, nextSeq: next, dropped, resets: base.resets, lastOffset: last }
}

// The number shown for a line: counted from the start of the follow.
export function lineNumber(line: HeldLine): number {
  return line.seq + 1
}

// ---- how fast to poll ----

export interface Pace {
  // How many quiet polls in a row (nothing new).
  quiet: number
  // How many failed polls in a row.
  failures: number
}

export const START_PACE: Pace = { quiet: 0, failures: 0 }

// While the log grows the next poll is a second away; while it is quiet the wait grows to five seconds; more waiting asks again at once.
export const ACTIVE_DELAY_MS = 1000
export const QUIET_DELAYS_MS = [1000, 2000, 3000, 5000]
export const ERROR_DELAYS_MS = [2000, 4000, 8000, 16000, 30000]
// A server that says "too many" is asked again no sooner than its own Retry-After (15 seconds) says.
export const TOO_MANY_DELAY_MS = 15000
// After this many failures in a row (about twenty minutes) the follow stops and says so.
export const MAX_FAILURES = 40

export function afterBatch(pace: Pace, batch: LiveBatch): { pace: Pace; delayMs: number } {
  if (batch.lines.length > 0 || batch.reset) {
    return { pace: START_PACE, delayMs: batch.eof ? ACTIVE_DELAY_MS : 0 }
  }
  const quiet = pace.quiet + 1
  return { pace: { quiet, failures: 0 }, delayMs: QUIET_DELAYS_MS[Math.min(quiet, QUIET_DELAYS_MS.length) - 1] }
}

// The HTTP status of a failure, when it carries one (an ApiError does): read without importing it, so the rules stay free of the client.
export function statusOf(err: unknown): number | undefined {
  if (typeof err === 'object' && err !== null && 'status' in err && typeof err.status === 'number') return err.status
  return undefined
}

// Statuses that retrying cannot fix: the session ended, no access, the log is gone, it is not text.
const FATAL = new Set([401, 403, 404, 415])

export function afterError(pace: Pace, err: unknown): { pace: Pace; delayMs: number; stop: boolean; message: string } {
  const status = statusOf(err)
  if (status !== undefined && FATAL.has(status)) {
    return { pace, delayMs: 0, stop: true, message: describeLiveFailure(status, false) }
  }
  const failures = pace.failures + 1
  if (failures >= MAX_FAILURES) {
    return {
      pace: { ...pace, failures },
      delayMs: 0,
      stop: true,
      message: 'The log could not be reached for a long time. Open it again to follow.',
    }
  }
  const step = ERROR_DELAYS_MS[Math.min(failures, ERROR_DELAYS_MS.length) - 1]
  const delayMs = status === 429 ? Math.max(step, TOO_MANY_DELAY_MS) : step
  return { pace: { ...pace, failures }, delayMs, stop: false, message: describeLiveFailure(status, true) }
}

// What to tell the person, by HTTP status. Never the server's own text: it is only ever shown as-is for the statuses we know.
export function describeLiveFailure(status: number | undefined, retrying: boolean): string {
  const again = retrying ? ' Trying again.' : ''
  switch (status) {
    case 401:
      return 'Your session has ended. Sign in again.'
    case 403:
      return 'You do not have access to this log.'
    case 404:
      return 'This log was not found. It may have been removed.'
    case 415:
      return 'This is not a text log.'
    case 429:
      return `Too many logs are followed at once. Close one to follow this.${again}`
    case 503:
      return `The log is busy right now.${again}`
    case 504:
      return `The log took too long to read.${again}`
    default:
      return `The log could not be read.${again}`
  }
}

// ---- what to say about what is held ----

// "following · 1,234 lines held · 120 older dropped · log restarted 2 times"
export function statusText(state: TailState, following: boolean): string {
  const n = state.lines.length
  const parts = [following ? 'following' : 'paused', `${n.toLocaleString('en-US')} ${n === 1 ? 'line' : 'lines'} held`]
  if (state.dropped > 0) parts.push(`${state.dropped.toLocaleString('en-US')} older dropped`)
  if (state.resets > 0) parts.push(state.resets === 1 ? 'log restarted' : `log restarted ${state.resets.toLocaleString('en-US')} times`)
  return parts.join(' · ')
}
