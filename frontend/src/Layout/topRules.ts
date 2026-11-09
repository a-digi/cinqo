import { formatBytes } from '../Shared/Components/Terminal/terminalRules'

// Pure rules for the memory dropdown (plan 226): the words and numbers it shows. No React, so they can be script-checked.

export const TOP_POLL_MS = 3000

export const FOOTNOTE =
  'Memory is the resident size of each program. Memory that programs share is counted in each, so the rows can add up to more than the computer has.'

// The share of the computer's memory, as a whole percent; "<1 %" below one.
export function shareText(bytes: number, totalBytes: number): string {
  if (totalBytes <= 0 || bytes <= 0) return '–'
  const p = (bytes / totalBytes) * 100
  return p < 1 ? '<1 %' : `${Math.round(p)} %`
}

// The width of a row's bar against the biggest row, 0 to 100.
export function barPercent(bytes: number, biggest: number): number {
  if (biggest <= 0 || bytes <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((bytes / biggest) * 100)))
}

// "PID 831" or "PID 831 · 36 processes".
export function pidText(pid: number, processes: number): string {
  return processes > 1 ? `PID ${pid} · ${processes} processes` : `PID ${pid}`
}

export function sizeText(bytes: number): string {
  return formatBytes(bytes)
}

export const LOADING_TEXT = 'Reading…'
export const EMPTY_TEXT = 'No programs were found.'
