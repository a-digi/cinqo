// The one cleaning every text read from or written to a list page's address goes through (step 135, 136): control
// characters become a space, the ends are trimmed, and the text is cut at MAX_TEXT_LENGTH characters (counted as
// characters, so an emoji is one). Shared so the pages' address rules cannot drift apart.

// The longest text kept, in characters.
export const MAX_TEXT_LENGTH = 200

export function cleanText(raw: string | null): string {
  if (raw === null) return ''
  // eslint-disable-next-line no-control-regex
  const spaced = raw.replace(/[\u0000-\u001F\u007F]+/g, ' ').trim()
  return Array.from(spaced).slice(0, MAX_TEXT_LENGTH).join('').trim()
}
