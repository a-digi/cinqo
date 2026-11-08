// crawlAiAdvice.ts (plan 213) — what the Platforms page recommends for the AI that prepares and runs a portal's or link's crawl.
// Pure (no React), so it can be script-checked. It is advice only: nothing here changes a choice.
//
// With an API key (a usable platform that needs one, and the key list really loaded) the advice is a premium model; without one the
// Local AI is what is used, and the advice is Qwen2.5-Coder-3B, the best local model Cinqo offers for this. The advice is about the
// choice in effect for that portal or link (its own, else its platform's, else the tool-wide one), so an inherited choice gets the
// advice for what it really uses.
import type { CrawlAiChoice } from '../../Cinqo/Platform/crawlAiChoice'
import { LOCAL_PLATFORM_ID, RECOMMENDED_LOCAL_CRAWL_MODEL } from '../../Cinqo/Platform/modelLabels'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import type { InstructionStats } from '../../api'

// How many times the instructions of a link must fail, with the best local model in use, before a premium model is advised.
export const PREMIUM_AFTER_FAILURES = 3

// The counter: how many times in a row the AI finished writing a link's instructions without saving any (the backend's own per-link
// count, instruction_attempts, step 87: a provider error or an interruption says nothing about the model and does not count). A kind
// whose instructions exist counts 0, the larger of the two kinds (job list, job details) is the link's.
export function linkInstructionFailures(link: {
  instructionStats: InstructionStats
  crawlInstructions?: string | null
  jobDetailCrawlInstructions?: string | null
}): number {
  const listing = (link.crawlInstructions ?? '').trim() !== '' ? 0 : link.instructionStats.listing.consecutiveFailures
  const detail = (link.jobDetailCrawlInstructions ?? '').trim() !== '' ? 0 : link.instructionStats.jobDetail.consecutiveFailures
  return Math.max(listing, detail)
}

export interface CrawlAiAdvice {
  // premium: a key exists, a premium model is not in use. qwen: no key, Coder 3B is ready but not in use. qwen-missing: no key, Coder 3B is
  // not downloaded. ok: the recommended kind of AI is already in use (shown quietly).
  kind: 'premium' | 'qwen' | 'qwen-missing' | 'ok'
  text: string
  // The button inside the pill: choose = open the AI modal with `suggest` preselected; get-model = go to the Local AI page; none = no button.
  action: 'choose' | 'get-model' | 'none'
  suggest: { platformId: string; model: string } | null
}

export function crawlAiAdvice(input: {
  usable: Platform[]
  keysKnown: boolean
  choice: CrawlAiChoice
  // linkInstructionFailures of the link (of a platform row: the largest of its links).
  failures: number
}): CrawlAiAdvice | null {
  const { usable, keysKnown, choice, failures } = input
  if (!choice.platform) return null
  const premium = keysKnown ? usable.find((p) => p.requiresKey !== false && p.models.length > 0) : undefined
  const effectiveIsLocal = choice.platform.id === LOCAL_PLATFORM_ID
  const usingQwen = effectiveIsLocal && choice.model === RECOMMENDED_LOCAL_CRAWL_MODEL

  if (premium) {
    if (!effectiveIsLocal) {
      return { kind: 'ok', text: 'Premium model in use', action: 'none', suggest: null }
    }
    // The best local model is in use: a premium model is advised only once the instructions have failed enough times, and the counter shows
    // how far it is.
    if (usingQwen && failures < PREMIUM_AFTER_FAILURES) {
      const counter = failures > 0 ? ` · failed ${failures} of ${PREMIUM_AFTER_FAILURES}` : ''
      return { kind: 'ok', text: `Best local model in use${counter}`, action: 'none', suggest: null }
    }
    // Only the platform is named, never a model: a platform's first listed model can be a poor one (OpenRouter's is the random free router,
    // which often fails at tool use), so the person picks the model in the dialog.
    return {
      kind: 'premium',
      text: usingQwen
        ? `Instructions failed ${failures} times: use a premium model, pick one from ${premium.name}`
        : `Use a premium model: pick one from ${premium.name}`,
      action: 'choose',
      suggest: { platformId: premium.id, model: '' },
    }
  }

  // No premium platform to offer: only the Local AI can be advised, and only when it is what this portal or link uses.
  if (!effectiveIsLocal) return null
  if (usingQwen) {
    const counter = failures > 0 ? ` · failed ${failures} time${failures === 1 ? '' : 's'}` : ''
    return { kind: 'ok', text: `Best local model in use${counter}`, action: 'none', suggest: null }
  }
  const local = usable.find((p) => p.id === LOCAL_PLATFORM_ID)
  if (local?.models.includes(RECOMMENDED_LOCAL_CRAWL_MODEL) === true) {
    return {
      kind: 'qwen',
      text: 'Use Qwen2.5-Coder-3B, the best local model for crawl instructions',
      action: 'choose',
      suggest: { platformId: local.id, model: RECOMMENDED_LOCAL_CRAWL_MODEL },
    }
  }
  return {
    kind: 'qwen-missing',
    text: 'Get Qwen2.5-Coder-3B, the best local model for crawl instructions',
    action: 'get-model',
    suggest: null,
  }
}
