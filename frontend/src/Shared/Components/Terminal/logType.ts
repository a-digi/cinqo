import type { FileView } from '../../../api/fileReader'

// The log type of a file in the viewer (plan 202): how the person wants it read. Text is the file as it is; JSON offers the beautifier.
export type LogType = 'text' | 'json'

export const LOG_TYPES: readonly LogType[] = ['text', 'json']

export const LOG_TYPE_LABEL: Record<LogType, string> = { text: 'Text', json: 'JSON' }

// The type a file opens as when nobody has chosen one: its name decides (a .json file opens as JSON, everything else as text). The content
// is never sniffed: a log that merely starts with "{" may be a line of something else.
export function defaultLogType(nameOrRef: string): LogType {
  const last = nameOrRef.split('/').pop() ?? nameOrRef
  return /\.json$/i.test(last) ? 'json' : 'text'
}

// Which view of the file the server is asked for: only the JSON type with Beautify on reads the indented copy.
export function viewFor(type: LogType, beautify: boolean): FileView {
  return type === 'json' && beautify ? 'json' : 'raw'
}
