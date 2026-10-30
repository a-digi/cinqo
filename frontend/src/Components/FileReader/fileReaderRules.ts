import { WINDOW_SIZES } from '../../Shared/Components/Terminal/terminalRules'

// What a tool may pass to bridge.mountFileReader, checked before anything is drawn or requested. The server
// checks it all again (source, scope, path), so this only keeps obvious mistakes from becoming requests.

export interface FileReaderMountOptions {
  tool: string
  source: string
  ref: string
  title?: string
  initialWindow?: number
}

const toolPattern = /^[a-z][a-z0-9_]*$/
const sourcePattern = /^[a-z][a-z0-9_-]{0,31}$/
const segmentPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/
const MAX_SEGMENTS = 6

// null when the options are fine, otherwise what is wrong (for a console message).
export function mountProblem(options: FileReaderMountOptions): string | null {
  if (typeof options.tool !== 'string' || !toolPattern.test(options.tool))
    return "tool must be the tool's slug (lower-case letters, digits and underscores)"
  if (typeof options.source !== 'string' || !sourcePattern.test(options.source))
    return 'source must be an id declared in manifest.json "file_sources"'
  if (typeof options.ref !== 'string' || options.ref === '') return 'ref is required'
  const parts = options.ref.split('/')
  if (parts.length > MAX_SEGMENTS || !parts.every((p) => segmentPattern.test(p))) {
    return 'ref must be a relative path of up to 6 parts made of letters, digits, dots, dashes and underscores (no "..", no leading dot)'
  }
  if (options.title !== undefined && typeof options.title !== 'string') return 'title must be text'
  if (options.initialWindow !== undefined && !WINDOW_SIZES.some((n) => n === options.initialWindow)) {
    return `initialWindow must be one of ${WINDOW_SIZES.join(', ')}`
  }
  return null
}

// The reader's source id for a tool's declared source.
export function toolSourceId(tool: string, source: string): string {
  return `tool:${tool}:${source}`
}
