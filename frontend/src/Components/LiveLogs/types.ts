// types.ts — what the live log module (plan/ai/file-reader/live-follow.md) knows about a log. A LiveFeed is the only seam: everything above it
// (the rules, the follow loop, the hook, later the viewer) asks "poll since this cursor" and never learns what is behind it, so a feed
// can be fed by another kind of log, or by something other than polling, without touching them.

// One line of a batch. `offset` is where the line starts in the log, in bytes (a live log has no line numbers: they would need a scan of
// the whole file); `text` is shorter than `bytes` when `truncated`.
export interface LiveLine {
  offset: number
  text: string
  bytes: number
  truncated?: boolean
}

// What one poll returns: the lines appended since the cursor (whole lines only), the cursor to ask with next, `reset` when the log was
// truncated or replaced so the lines start over, and `eof` when nothing more is waiting right now (false: ask again at once).
export interface LiveBatch {
  lines: LiveLine[]
  cursor: string
  reset: boolean
  eof: boolean
}

// A kind of log the server can follow, and the title to show for it.
export interface LiveProvider {
  id: string
  title: string
}

// Something that can be polled. An undefined cursor means "start at the end": the last lines, then follow. A failure is thrown as an
// Error that may carry a numeric `status` (an HTTP status or anything that means the same); the loop decides what to do with it.
export interface LiveFeed {
  poll: (cursor: string | undefined, signal: AbortSignal) => Promise<LiveBatch>
}
