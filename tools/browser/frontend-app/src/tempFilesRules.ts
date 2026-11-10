// Pure rules for the "Temporary files" card (plan 248): the words and numbers it shows. No React, so they can be script-checked.
import type { TempFilesCleared, TempFilesPart, TempFilesResponse } from './api'
import { plural } from './Shared/plural'

// A size for people: 640 B, 12.5 KB, 3.1 MB, 1.2 GB.
export function formatBytes(n: number): string {
  const b = Math.max(0, n)
  if (b < 1024) return `${String(Math.round(b))} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(1)} GB`
}

// What a part is, in one line: "2 files · 668.7 KB", with "· 1 in use" for browser profiles of a browser that runs.
export function partText(p: TempFilesPart): string {
  const base = `${plural(p.files, 'file', 'files')} · ${formatBytes(p.bytes)}`
  return p.inUse !== undefined && p.inUse > 0 ? `${base} · ${String(p.inUse)} in use` : base
}

// Below this the "other" part is only the list of running browsers (a few bytes), which is never deleted and is explained in a sentence instead.
export const OTHER_NOTE_LIMIT = 1024

// The parts to list: the three known ones always (so the page reads the same when they are empty), "other" only when there is more in it than the list
// of running browsers.
export function visibleParts(parts: TempFilesPart[]): TempFilesPart[] {
  return parts.filter((p) => p.id !== 'other' || p.bytes > OTHER_NOTE_LIMIT)
}

// Whether a delete would do anything: something other than the list of running browsers is in the folder.
export function canDelete(data: TempFilesResponse): boolean {
  return data.parts.some((p) => (p.id === 'other' ? p.bytes > OTHER_NOTE_LIMIT : p.files > 0))
}

export const EXPLANATION =
  'Page copies, the page cache and browser profiles are made while the tool works and can always be made again. They are removed automatically when ' +
  'the tool starts and stops and when a new page copy is made, so you never have to.'

export const KEPT_NOTE = 'The list of running browsers (a few bytes) is always kept.'

export function deleteQuestion(bytes: number): string {
  return `Delete ${formatBytes(bytes)} of temporary files? A browser that is running keeps what it uses.`
}

// What a delete just did, in a sentence.
export function clearedText(r: TempFilesCleared): string {
  if (r.deletedFiles === 0) return 'There was nothing to delete.'
  const done = `Deleted ${plural(r.deletedFiles, 'file', 'files')} (${formatBytes(r.deletedBytes)}).`
  return r.keptReason !== '' ? `${done} ${r.keptReason.charAt(0).toUpperCase()}${r.keptReason.slice(1)}.` : done
}
