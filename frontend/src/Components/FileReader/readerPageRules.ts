import { mountProblem } from './fileReaderRules'

// The address of the reader page (plan 201): a tool sends the person to /logs/read with everything in the query string, so a reload, a
// bookmark and a link in a new tab all behave alike. Pure: no React, no requests. The server checks the source, the scope and every path
// again; this only keeps obvious mistakes from becoming requests, and keeps the return link on this site.
//
//   /logs/read?tool=browser&source=crawl-logs&ref=crawl_logs/a.json&ref=crawl_logs/b.json
//             &open=crawl_logs/b.json&format=json&title=Crawl%20logs&back=/tools/browser/crawl-logs&backLabel=Crawl%20Logs

export const READER_PAGE_PATH = '/logs/read'

export const MAX_REFS = 20
export const MAX_TITLE = 80
export const MAX_BACK_LABEL = 40
export const MAX_BACK = 300
export const DEFAULT_BACK_LABEL = 'Back'
// The whole address: at most 20 paths of up to 6 x 128 characters would be far more than any real link.
const MAX_QUERY = 4000

// How the files open to begin with (plan 202): as text, or as JSON with the beautifier on. Left out, a file's name decides (a .json file opens
// as JSON); the person can always switch in the viewer.
export type ReaderFormat = 'text' | 'json'

export function isReaderFormat(value: unknown): value is ReaderFormat {
  return value === 'text' || value === 'json'
}

export interface ReaderPageOptions {
  tool: string
  source: string
  refs: string[]
  format?: ReaderFormat
  title?: string
  // Which of the refs opens first (default the first).
  open?: string
  // A path on this site to go back to, and what to call it.
  back?: string
  backLabel?: string
}

export interface ReaderPageView {
  tool: string
  source: string
  refs: string[]
  open: string
  // undefined: the file name decides.
  format: ReaderFormat | undefined
  // The page heading ('' = the default one).
  title: string
  // null when no usable return link was given.
  back: string | null
  backLabel: string
}

export type ParsedReaderQuery =
  | { ok: true; view: ReaderPageView }
  // A bad address: what is wrong, plus the return link when that part was fine (the page always offers a way back when it can).
  | { ok: false; message: string; back: string | null; backLabel: string }

// Text for a heading or a link label: control characters removed, spaces folded, cut to max characters. It is only ever shown as text.
export function cleanText(raw: string | null | undefined, max: number): string {
  const text = (raw ?? '')
    // eslint-disable-next-line no-control-regex -- the point is to drop control characters
    .replace(/[\u0000-\u001f\u007f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  return Array.from(text).slice(0, max).join('')
}

const PROBE = 'http://cinqo.invalid'

// A return link is accepted only as a path on this site: it starts with one "/", has no backslash or control character, stays on this
// origin when resolved, is not too long, and is not the reader page itself. Anything else is null (no link is drawn, nothing redirects).
export function safeBackPath(raw: string | null | undefined): string | null {
  if (typeof raw !== 'string' || raw === '' || raw.length > MAX_BACK) return null
  if (!raw.startsWith('/') || raw.startsWith('//')) return null
  if (raw.includes('\\')) return null
  // eslint-disable-next-line no-control-regex -- the point is to refuse control characters
  if (/[\u0000-\u001f\u007f]/.test(raw)) return null
  let url: URL
  try {
    url = new URL(raw, PROBE)
  } catch {
    return null
  }
  if (url.origin !== PROBE) return null
  if (url.pathname === READER_PAGE_PATH || url.pathname.startsWith(READER_PAGE_PATH + '/')) return null
  return raw
}

function problemOf(tool: string, source: string, refs: string[]): string | null {
  if (refs.length === 0) return 'no file was given'
  if (refs.length > MAX_REFS) return `at most ${MAX_REFS} files can be opened at once`
  if (new Set(refs).size !== refs.length) return 'a file is listed twice'
  for (const ref of refs) {
    const p = mountProblem({ tool, source, ref })
    if (p) return p
  }
  return null
}

// Reads the page's query string. The tool, the source and every ref are checked (the same rules as bridge.mountFileReader); the
// heading and the label are only cleaned.
export function parseReaderQuery(search: string): ParsedReaderQuery {
  const params = new URLSearchParams(search)
  const back = safeBackPath(params.get('back'))
  const backLabel = cleanText(params.get('backLabel'), MAX_BACK_LABEL) || DEFAULT_BACK_LABEL
  const fail = (message: string): ParsedReaderQuery => ({ ok: false, message, back, backLabel })

  if (search.length > MAX_QUERY) return fail('the address is too long')
  const tool = params.get('tool') ?? ''
  const source = params.get('source') ?? ''
  const refs = params.getAll('ref')
  const problem = problemOf(tool, source, refs)
  if (problem) return fail(problem)

  const wanted = params.get('open')
  const open = wanted !== null && refs.includes(wanted) ? wanted : refs[0]
  // The format only decides how the viewer starts, so a value this page does not know is ignored (the viewer's own switch is always there)
  // rather than refusing the whole link.
  const asked = params.get('format')
  const format = isReaderFormat(asked) ? asked : undefined
  return { ok: true, view: { tool, source, refs, open, format, title: cleanText(params.get('title'), MAX_TITLE), back, backLabel } }
}

// The reader page's address for these options, or the first thing wrong with them (what bridge.openFileReader and fileReaderPath use).
export function buildReaderPath(options: ReaderPageOptions): { ok: true; path: string } | { ok: false; message: string } {
  const problem = problemOf(options.tool, options.source, options.refs)
  if (problem) return { ok: false, message: problem }
  if (options.open !== undefined && !options.refs.includes(options.open)) return { ok: false, message: 'open must be one of the files' }
  if (options.format !== undefined && !isReaderFormat(options.format)) return { ok: false, message: 'format must be "text" or "json"' }
  if (options.back !== undefined && safeBackPath(options.back) === null) {
    return { ok: false, message: 'back must be a path on this site (starting with a single "/")' }
  }
  const params = new URLSearchParams()
  params.set('tool', options.tool)
  params.set('source', options.source)
  for (const ref of options.refs) params.append('ref', ref)
  if (options.open !== undefined) params.set('open', options.open)
  if (options.format !== undefined) params.set('format', options.format)
  const title = cleanText(options.title, MAX_TITLE)
  if (title) params.set('title', title)
  if (options.back !== undefined) {
    params.set('back', options.back)
    const label = cleanText(options.backLabel, MAX_BACK_LABEL)
    if (label) params.set('backLabel', label)
  }
  const query = params.toString()
  if (query.length > MAX_QUERY) return { ok: false, message: 'the address would be too long' }
  return { ok: true, path: `${READER_PAGE_PATH}?${query}` }
}

// The file's name for the list: the last part of its path.
export function fileName(ref: string): string {
  const parts = ref.split('/')
  return parts[parts.length - 1] ?? ref
}

// What a tool passes to bridge.openFileReader / fileReaderPath: the reader page's options, where the return link is optional. Left out, the
// person comes back to the page they are on now (a tool does not have to build its own address); a page that cannot be returned to (the
// reader itself, or an address that is not a plain path) simply gets no link. backLabel defaults to "Back" when the page parses it.
export function withReturnDefaults(options: ReaderPageOptions, currentPath: string): ReaderPageOptions {
  if (options.back !== undefined) return options
  const here = safeBackPath(currentPath)
  return here === null ? options : { ...options, back: here }
}
