// followLoop.ts — the loop that polls a LiveFeed one batch after the other, apart from React so it can be script-checked with a fake feed and a
// fake clock. It never overlaps two polls (each waits for the last), stops at once when its signal aborts, waits while the tab is hidden,
// slows down while the log is quiet, backs off after a failure, and gives up on a failure that retrying cannot fix.
import { afterBatch, afterError, START_PACE, type Pace } from './tailRules'
import type { LiveBatch, LiveFeed } from './types'

export type LoopState =
  { kind: 'following' } | { kind: 'hidden' } | { kind: 'retrying'; message: string; inMs: number } | { kind: 'stopped'; message: string }

export interface LoopDeps {
  feed: LiveFeed
  signal: AbortSignal
  // Where to continue (a loop that was paused and is resumed); undefined starts at the end of the log.
  startCursor?: string
  // Takes a batch in. Called once per successful poll, in order.
  apply: (batch: LiveBatch) => void
  // The cursor to continue from, after each successful poll.
  onCursor: (cursor: string) => void
  onState: (state: LoopState) => void
  // Waits ms, and resolves early when the signal aborts.
  sleep: (ms: number, signal: AbortSignal) => Promise<void>
  // Resolves at once while the tab is visible, otherwise when it becomes visible (or the signal aborts).
  waitVisible: (signal: AbortSignal) => Promise<void>
  isVisible: () => boolean
}

export async function followLoop(d: LoopDeps): Promise<void> {
  // a function, so the checks after each await are not narrowed to "never aborted"
  const aborted = () => d.signal.aborted
  let cursor = d.startCursor
  let pace: Pace = START_PACE
  while (!aborted()) {
    if (!d.isVisible()) {
      d.onState({ kind: 'hidden' })
      await d.waitVisible(d.signal)
      continue
    }
    d.onState({ kind: 'following' })
    let batch: LiveBatch
    try {
      batch = await d.feed.poll(cursor, d.signal)
    } catch (err) {
      if (aborted()) return
      const next = afterError(pace, err)
      pace = next.pace
      if (next.stop) {
        d.onState({ kind: 'stopped', message: next.message })
        return
      }
      d.onState({ kind: 'retrying', message: next.message, inMs: next.delayMs })
      await d.sleep(next.delayMs, d.signal)
      continue
    }
    if (aborted()) return
    cursor = batch.cursor
    d.apply(batch)
    d.onCursor(cursor)
    const next = afterBatch(pace, batch)
    pace = next.pace
    if (next.delayMs > 0) await d.sleep(next.delayMs, d.signal)
  }
}
