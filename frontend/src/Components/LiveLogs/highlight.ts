// highlight.ts — colours the structure of an AI trace log in the live viewer. Its own copy of the Terminal viewer's rule (the two viewers
// share no code, so either can be removed or changed alone): the "=== Iteration N — request / response / error ===" banners, the
// "[n] role=…" message heads, tool calls, and the footers.
export type HighlightKind = 'section' | 'error' | 'role' | 'tool' | 'meta'
export type Highlighter = (text: string) => HighlightKind | null

export const traceLogHighlight: Highlighter = (text) => {
  if (text.startsWith('=== ')) return /\berror\b/i.test(text) ? 'error' : 'section'
  if (/^\[\d+\] role=/.test(text)) return 'role'
  if (text.startsWith('tool_call:')) return 'tool'
  if (text.startsWith('finish_reason=') || text.startsWith('--- tools offered') || text.startsWith('requests=')) return 'meta'
  return null
}

export const HIGHLIGHT_CLASS: Record<HighlightKind, string> = {
  section: 'font-semibold text-sky-300',
  error: 'font-semibold text-red-400',
  role: 'text-emerald-300',
  tool: 'text-amber-200',
  meta: 'text-[#8b95a5]',
}
