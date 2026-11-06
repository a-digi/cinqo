// engineSuggestion.ts (plan 197, step 4) — the words of the suggestion to read a link with Rod when the site stopped Katana. Pure: the code is
// the Browser's error code, nothing else of the site's answer is shown. The suggestion is offered, never applied by itself.

export const USE_ROD_LABEL = 'Use Rod for this link'
export const NOT_NOW_LABEL = 'Not now'

// engineSuggestionText: what stopped Katana, and what Rod can do about it. A rate limit (429) is "try again later" first, because waiting is
// often the real answer; the other codes are a check or a wall that Rod can ask the person to clear in a window.
export function engineSuggestionText(code: string): string {
  switch (code) {
    case 'rate_limited':
      return 'The site rate-limited Katana (too many requests). Try again later, or use Rod for this link.'
    case 'bot_protection_detected':
      return 'A bot-protection wall stopped Katana here. Rod can ask you to clear it in a window.'
    case 'cloudflare_blocked':
      return 'Cloudflare blocked Katana here. Rod can read this link instead, and ask you to clear a check in a window if there is one.'
    default:
      return 'Cloudflare stopped Katana with a check it could not clear. Rod can ask you to clear it in a window.'
  }
}

// What Rod does and does not change, said once under the line (a blocked job-detail crawl is the same path for both engines).
export const ENGINE_SUGGESTION_NOTE =
  'Rod reads the job list and the pages the AI looks at. A job-detail crawl that the site blocks is handled the same way by both engines.'
