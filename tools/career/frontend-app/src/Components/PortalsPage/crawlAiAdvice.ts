// crawlAiAdvice.ts (plan 213) — what the Platforms page recommends for the AI that prepares and runs a portal's or link's crawl.
// Pure (no React), so it can be script-checked. It is advice only: nothing here changes a choice.
//
// With an API key (a usable platform that needs one, and the key list really loaded) the advice is a premium model; without one the
// Local AI is what is used, and the advice is Qwen2.5-Coder-3B, the best local model Cinqo offers for this. The advice is about the
// choice in effect for that portal or link (its own, else its platform's, else the tool-wide one), so an inherited choice gets the
// advice for what it really uses.
import type { CrawlAiChoice } from '../../Cinqo/Platform/crawlAiChoice'
import { defaultModelFor } from '../../Cinqo/Platform/aiChoice'
import { LOCAL_PLATFORM_ID, RECOMMENDED_LOCAL_CRAWL_MODEL, modelLabel } from '../../Cinqo/Platform/modelLabels'
import type { Platform } from '../../Cinqo/Platform/platformRepository'

export interface CrawlAiAdvice {
  // premium: a key exists, a premium model is not in use. qwen: no key, Coder 3B is ready but not in use. qwen-missing: no key, Coder 3B is
  // not downloaded. ok: the recommended kind of AI is already in use (shown quietly).
  kind: 'premium' | 'qwen' | 'qwen-missing' | 'ok'
  text: string
  // The button inside the pill: choose = open the AI modal with `suggest` preselected; get-model = go to the Local AI page; none = no button.
  action: 'choose' | 'get-model' | 'none'
  suggest: { platformId: string; model: string } | null
}

export function crawlAiAdvice(input: { usable: Platform[]; keysKnown: boolean; choice: CrawlAiChoice }): CrawlAiAdvice | null {
  const { usable, keysKnown, choice } = input
  if (!choice.platform) return null
  const premium = keysKnown ? usable.find((p) => p.requiresKey !== false && p.models.length > 0) : undefined
  const effectiveIsLocal = choice.platform.id === LOCAL_PLATFORM_ID

  if (premium) {
    if (!effectiveIsLocal) {
      return { kind: 'ok', text: 'Premium model in use', action: 'none', suggest: null }
    }
    const model = defaultModelFor(premium)
    return {
      kind: 'premium',
      text: `Use a premium model: ${premium.name} · ${modelLabel(premium, model)}`,
      action: 'choose',
      suggest: { platformId: premium.id, model },
    }
  }

  // No premium platform to offer: only the Local AI can be advised, and only when it is what this portal or link uses.
  if (!effectiveIsLocal) return null
  if (choice.model === RECOMMENDED_LOCAL_CRAWL_MODEL) {
    return { kind: 'ok', text: 'Best local model in use', action: 'none', suggest: null }
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
