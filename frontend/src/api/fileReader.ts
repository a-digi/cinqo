import { apiGet } from './client'

// The file reader (plan/ai/file-reader/file-reader-engine.md): a big text file read a window of lines at a
// time. A file is named by a source id and a reference, never by a path; the server decides what they mean
// and whether the caller may read it.

// Which view of a file to read: as it is, or (plan 202) its JSON beautified. The server makes the indented copy once and reads it like any file.
export type FileView = 'raw' | 'json'

export interface FileInfo {
  sizeBytes: number
  modifiedAt: string
  scannedBytes: number
  // Exact once `complete`, otherwise a lower bound that grows while the server indexes the file.
  totalLines: number
  complete: boolean
  // Set for the JSON view: which view this is and how big the original file is (sizeBytes is then the indented copy's).
  view?: FileView
  sourceBytes?: number
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

function query(ref: string, extra: Record<string, number> = {}, view: FileView = 'raw'): string {
  const params = new URLSearchParams({ ref })
  for (const [k, v] of Object.entries(extra)) params.set(k, String(v))
  if (view === 'json') params.set('view', 'json')
  return params.toString()
}

export async function fetchFileInfo(source: string, ref: string, signal?: AbortSignal, view: FileView = 'raw'): Promise<FileInfo> {
  const raw = await apiGet<{ message: FileInfo }>(`/api/v1/file-reader/${encodeURIComponent(source)}/info?${query(ref, {}, view)}`, signal)
  return raw.message
}

export async function fetchFileLines(
  source: string,
  ref: string,
  from: number,
  count: number,
  signal?: AbortSignal,
  view: FileView = 'raw',
): Promise<FileWindow> {
  const raw = await apiGet<{ message: FileWindow }>(
    `/api/v1/file-reader/${encodeURIComponent(source)}/lines?${query(ref, { from, count }, view)}`,
    signal,
  )
  return raw.message
}

export interface SearchMatch {
  n: number
  // How many bytes into the line the match starts.
  offset: number
  preview: string
}

export interface SearchResult {
  matches: SearchMatch[]
  // Where to ask next when `done` is false: a line and a byte offset inside it.
  nextFrom: number
  nextOffset: number
  done: boolean
  scannedBytes: number
}

export async function fetchFileSearch(
  source: string,
  ref: string,
  text: string,
  from: number,
  offset: number,
  caseSensitive: boolean,
  signal?: AbortSignal,
  view: FileView = 'raw',
): Promise<SearchResult> {
  const params = new URLSearchParams({ ref, q: text, from: String(from), offset: String(offset) })
  if (caseSensitive) params.set('case', '1')
  if (view === 'json') params.set('view', 'json')
  const raw = await apiGet<{ message: SearchResult }>(
    `/api/v1/file-reader/${encodeURIComponent(source)}/search?${params.toString()}`,
    signal,
  )
  return raw.message
}

// One piece of a single line, read by byte offset: how a line longer than a window shows is read to its end.
export interface FileSegment {
  line: number
  // Where the text really starts in the line (moved forward past the middle of a character).
  offset: number
  text: string
  // Where the next segment starts; always on a character boundary.
  nextOffset: number
  // The line's whole length in bytes.
  lineBytes: number
  hasMore: boolean
}

export async function fetchFileSegment(
  source: string,
  ref: string,
  line: number,
  offset: number,
  signal?: AbortSignal,
  view: FileView = 'raw',
): Promise<FileSegment> {
  const params = new URLSearchParams({ ref, line: String(line), offset: String(offset) })
  if (view === 'json') params.set('view', 'json')
  const raw = await apiGet<{ message: FileSegment }>(
    `/api/v1/file-reader/${encodeURIComponent(source)}/segment?${params.toString()}`,
    signal,
  )
  return raw.message
}
