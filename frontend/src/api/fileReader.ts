import { apiGet } from './client'

// The file reader (plan/ai/file-reader/file-reader-engine.md): a big text file read a window of lines at a
// time. A file is named by a source id and a reference, never by a path; the server decides what they mean
// and whether the caller may read it.

export interface FileInfo {
  sizeBytes: number
  modifiedAt: string
  scannedBytes: number
  // Exact once `complete`, otherwise a lower bound that grows while the server indexes the file.
  totalLines: number
  complete: boolean
}

export interface FileLine {
  n: number
  text: string
  // The line's real length in bytes; `text` is shorter when `truncated`.
  bytes: number
  truncated?: boolean
}

export interface FileWindow {
  from: number
  lines: FileLine[]
  nextFrom: number
  hasMore: boolean
}

function query(ref: string, extra: Record<string, number> = {}): string {
  const params = new URLSearchParams({ ref })
  for (const [k, v] of Object.entries(extra)) params.set(k, String(v))
  return params.toString()
}

export async function fetchFileInfo(source: string, ref: string, signal?: AbortSignal): Promise<FileInfo> {
  const raw = await apiGet<{ message: FileInfo }>(`/api/v1/file-reader/${encodeURIComponent(source)}/info?${query(ref)}`, signal)
  return raw.message
}

export async function fetchFileLines(source: string, ref: string, from: number, count: number, signal?: AbortSignal): Promise<FileWindow> {
  const raw = await apiGet<{ message: FileWindow }>(
    `/api/v1/file-reader/${encodeURIComponent(source)}/lines?${query(ref, { from, count })}`,
    signal,
  )
  return raw.message
}

export interface SearchMatch {
  n: number
  preview: string
}

export interface SearchResult {
  matches: SearchMatch[]
  // Where to ask next when `done` is false.
  nextFrom: number
  done: boolean
  scannedBytes: number
}

export async function fetchFileSearch(
  source: string,
  ref: string,
  text: string,
  from: number,
  caseSensitive: boolean,
  signal?: AbortSignal,
): Promise<SearchResult> {
  const params = new URLSearchParams({ ref, q: text, from: String(from) })
  if (caseSensitive) params.set('case', '1')
  const raw = await apiGet<{ message: SearchResult }>(
    `/api/v1/file-reader/${encodeURIComponent(source)}/search?${params.toString()}`,
    signal,
  )
  return raw.message
}
