import type { FileLine } from '../../../api/fileReader'

// The pure rules behind the terminal viewer (plan/ai/file-reader/file-reader-engine.md): how a window of
// lines moves over a file, how it is described, and what a key press does. No React, so they can be
// script-checked.

export const WINDOW_SIZES = [50, 100, 200, 500] as const
export type WindowSize = (typeof WINDOW_SIZES)[number]
export const DEFAULT_WINDOW: WindowSize = 200

// The first line of the last full window: where "End" goes.
export function lastFrom(total: number, size: number): number {
  return Math.max(1, total - size + 1)
}

// Keeps a window start inside the file: from 1 up to the start of the last full window. With an unknown
// (still growing) total the upper bound is the best known one.
export function clampFrom(from: number, total: number, size: number): number {
  if (!Number.isFinite(from)) return 1
  return Math.min(Math.max(1, Math.trunc(from)), lastFrom(total, size))
}

// A window one page further (+1) or back (-1).
export function pageFrom(from: number, direction: 1 | -1, total: number, size: number): number {
  return clampFrom(from + direction * size, total, size)
}

// The window that shows line `line`: it starts at that line, pulled back when that would run past the end
// so the window stays full.
export function jumpFrom(line: number, total: number, size: number): number {
  return clampFrom(line, total, size)
}

// "1,234" or "1234" or " 12 " as a line number, or null when it is not a positive whole number. Anything
// beyond a known total is pulled back to the last line.
export function parseLineInput(text: string, total: number): number | null {
  const cleaned = text.replace(/[,\s_]/g, '')
  if (!/^\d{1,15}$/.test(cleaned)) return null
  const n = Number(cleaned)
  if (n < 1) return null
  return total > 0 ? Math.min(n, total) : n
}

export function formatCount(n: number): string {
  return Math.trunc(n).toLocaleString('en-US')
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  return `${value >= 100 || Number.isInteger(value) ? Math.round(value) : value.toFixed(1)} ${units[i]}`
}

// Percent of the file the server has indexed so far, 0 to 100.
export function indexProgress(scannedBytes: number, sizeBytes: number): number {
  if (sizeBytes <= 0) return 100
  return Math.min(100, Math.floor((scannedBytes / sizeBytes) * 100))
}

export interface StatusInput {
  from: number
  shown: number
  total: number
  complete: boolean
  sizeBytes: number
}

// "lines 1,201–1,400 of 6,215 · 1.2 MB". While the index is still growing the total is a lower bound and
// carries a "+".
export function statusText({ from, shown, total, complete, sizeBytes }: StatusInput): string {
  const size = formatBytes(sizeBytes)
  if (shown === 0) return `no lines · ${size}`
  const to = from + shown - 1
  const of = `${formatCount(total)}${complete ? '' : '+'}`
  return `lines ${formatCount(from)}–${formatCount(to)} of ${of} · ${size}`
}

export interface Thumb {
  // Both are fractions of the track, 0 to 1.
  top: number
  height: number
}

const MIN_THUMB = 0.02

// Where the window sits on the minimap bar that stands for the whole file.
export function thumb(from: number, shown: number, total: number): Thumb {
  if (total <= 0 || shown <= 0) return { top: 0, height: 1 }
  const height = Math.min(1, Math.max(MIN_THUMB, shown / total))
  const top = Math.min(1 - height, Math.max(0, (from - 1) / total))
  return { top, height }
}

// A click or drag at `fraction` of the track (0 top, 1 bottom) centres the window there.
export function fromFraction(fraction: number, total: number, size: number): number {
  const f = Math.min(1, Math.max(0, Number.isFinite(fraction) ? fraction : 0))
  return clampFrom(Math.round(f * total - size / 2) + 1, total, size)
}

// How many bytes of a cut line are not shown (the text is plain characters, the length is in bytes).
export function hiddenBytes(line: FileLine, encodedLength: (text: string) => number): number {
  if (!line.truncated) return 0
  return Math.max(0, line.bytes - encodedLength(line.text))
}

export type Move =
  | { kind: 'page'; direction: 1 | -1 }
  | { kind: 'edge'; edge: 'top' | 'bottom' }
  // scroll the pane itself by this many lines, without changing the window
  | { kind: 'scroll'; lines: 1 | -1 }
  // `/` opens the search box; n and N go to the next and the previous match
  | { kind: 'search' }
  | { kind: 'match'; direction: 1 | -1 }

// The window-level keys, plus the usual `less` ones (space / f and b page, g and G go to the ends, j and k
// scroll a line, / searches, n and N step through the matches). The arrow keys are left alone: they scroll
// inside the pane like in any terminal. Callers only use this for keys pressed on the pane itself, never in
// an input, and never with Ctrl, Alt or Cmd.
export function keyToMove(key: string, shift = false): Move | null {
  switch (key) {
    case 'PageDown':
    case 'f':
      return { kind: 'page', direction: 1 }
    case ' ':
      return { kind: 'page', direction: shift ? -1 : 1 }
    case 'PageUp':
    case 'b':
      return { kind: 'page', direction: -1 }
    case 'Home':
    case 'g':
      return { kind: 'edge', edge: 'top' }
    case 'End':
    case 'G':
      return { kind: 'edge', edge: 'bottom' }
    case 'j':
      return { kind: 'scroll', lines: 1 }
    case 'k':
      return { kind: 'scroll', lines: -1 }
    case '/':
      return { kind: 'search' }
    case 'n':
      return { kind: 'match', direction: 1 }
    case 'N':
      return { kind: 'match', direction: -1 }
    default:
      return null
  }
}

export const MAX_QUERY_BYTES = 200

// Whether a search text may be sent: not empty, at most 200 bytes, no NUL. (Spaces count: " the " is a search.)
export function isSearchable(query: string, encodedLength: (text: string) => number): boolean {
  return query !== '' && !query.includes('\u0000') && encodedLength(query) <= MAX_QUERY_BYTES
}

export interface Part {
  text: string
  hit: boolean
}

const MAX_PARTS = 200

// Splits a line into plain and matching parts, so the matches can be marked. Plain text only (no pattern),
// case ignored unless asked for. A line whose lower-case form changes length (a few exotic letters) is left
// unmarked rather than marked in the wrong place.
export function splitMatches(text: string, query: string, caseSensitive: boolean): Part[] {
  if (query === '') return [{ text, hit: false }]
  const hay = caseSensitive ? text : text.toLowerCase()
  const needle = caseSensitive ? query : query.toLowerCase()
  if (hay.length !== text.length || needle.length !== query.length) return [{ text, hit: false }]
  const parts: Part[] = []
  let from = 0
  for (;;) {
    const at = hay.indexOf(needle, from)
    if (at < 0 || parts.length >= MAX_PARTS) break
    if (at > from) parts.push({ text: text.slice(from, at), hit: false })
    parts.push({ text: text.slice(at, at + needle.length), hit: true })
    from = at + needle.length
  }
  if (parts.length === 0) return [{ text, hit: false }]
  if (from < text.length) parts.push({ text: text.slice(from), hit: false })
  return parts
}

// Which match to start on after a search: the first one at or after the line the person was looking at,
// otherwise the first of all. -1 when there is none.
export function startMatch(lines: number[], anchor: number): number {
  if (lines.length === 0) return -1
  const i = lines.findIndex((n) => n >= anchor)
  return i < 0 ? 0 : i
}

// The next match to go to from `index`: the following one, or the previous one. Stays put at the ends of
// what is loaded; the caller asks for more first when `direction` is 1 at the end and the search is not done.
export function stepMatch(index: number, count: number, direction: 1 | -1): number {
  if (count === 0) return -1
  return Math.min(count - 1, Math.max(0, index + direction))
}

export interface MatchStatusInput {
  count: number
  index: number
  done: boolean
  searching: boolean
  failed: boolean
}

// "match 3 of 50+", "no matches", "searching…".
export function matchStatus({ count, index, done, searching, failed }: MatchStatusInput): string {
  if (failed) return 'search failed'
  if (searching && count === 0) return 'searching…'
  if (count === 0) return 'no matches'
  return `match ${formatCount(index + 1)} of ${formatCount(count)}${done ? '' : '+'}`
}

// The prompt line at the foot of the terminal: `$ view <name> --lines 1201-1400`.
export function promptText(name: string, from: number, shown: number): string {
  const label = name.trim() === '' ? 'file' : name.trim().replace(/\s+/g, '-')
  return shown > 0 ? `$ view ${label} --lines ${from}-${from + shown - 1}` : `$ view ${label}`
}

export type HighlightKind = 'section' | 'error' | 'role' | 'tool' | 'meta'
export type Highlighter = (text: string) => HighlightKind | null

// Colours the structure of an AI trace log (step 36): the "=== Iteration N — request / response / error ==="
// banners, the "[n] role=…" message heads, tool calls and the "finish_reason" / "tools offered" footers.
export const traceLogHighlight: Highlighter = (text) => {
  if (text.startsWith('=== ')) return /\berror\b/i.test(text) ? 'error' : 'section'
  if (/^\[\d+\] role=/.test(text)) return 'role'
  if (text.startsWith('tool_call:')) return 'tool'
  if (text.startsWith('finish_reason=') || text.startsWith('--- tools offered')) return 'meta'
  return null
}

// What to tell the person when the reader fails, by HTTP status. Never the server's own text: it is only
// ever shown as-is for the statuses we know.
export function describeFailure(status: number | undefined): string {
  switch (status) {
    case 401:
      return 'Your session has ended. Sign in again.'
    case 403:
      return 'You do not have access to this file.'
    case 404:
      return 'This file was not found. It may have been removed.'
    case 413:
      return 'This file is too large to read here.'
    case 415:
      return 'This is not a text file.'
    case 503:
      return 'The reader is busy right now. Try again in a moment.'
    default:
      return 'The file could not be read.'
  }
}
