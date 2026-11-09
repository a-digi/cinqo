// Pure rules of the How To pages (plan/ai/frontend/step-219-how-to-landing-pages.md), kept apart from React so they can be script-checked.

export const HOW_TO_BASE = '/admin/ai/how-to'

// The pages in the order of the slider and of the menu: the overview, the local agents, then the platforms that need a key.
export const HOW_TO_PAGE_IDS = ['overview', 'local-agents', 'openrouter', 'openai', 'anthropic'] as const

export type HowToPageId = (typeof HOW_TO_PAGE_IDS)[number]

export function howToPath(pageId: string): string {
  return `${HOW_TO_BASE}/${encodeURIComponent(pageId)}`
}

export function isKnownPageId(pageId: string): pageId is HowToPageId {
  return (HOW_TO_PAGE_IDS as readonly string[]).includes(pageId)
}

// The page before and after one, for the "Next" row; null at the ends. A page that is not one of the five (a platform added to the registry later)
// has no neighbours: it is reached from its slide only.
export function neighbours(pageId: string): { previous: HowToPageId | null; next: HowToPageId | null } {
  const i = (HOW_TO_PAGE_IDS as readonly string[]).indexOf(pageId)
  if (i === -1) return { previous: null, next: null }
  return { previous: i > 0 ? HOW_TO_PAGE_IDS[i - 1] : null, next: i < HOW_TO_PAGE_IDS.length - 1 ? HOW_TO_PAGE_IDS[i + 1] : null }
}

// Words that must never appear in an editorial text: nothing in the code can verify a price or a free-credit promise (see platformInfo.ts).
const FORBIDDEN = [/[$€£]/, /\bper month\b/i, /\bfree credits?\b/i, /\bfree trial\b/i, /\bunlimited\b/i, /\bguarantee/i, /\bcheapest\b/i]

export function forbiddenIn(text: string): string | null {
  for (const re of FORBIDDEN) {
    const m = re.exec(text)
    if (m) return m[0]
  }
  return null
}
