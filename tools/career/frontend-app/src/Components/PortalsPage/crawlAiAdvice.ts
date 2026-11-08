// crawlAiAdvice.ts (plan 213) — what the Platforms page recommends for the AI that prepares and runs a link's crawl (links only, never the platform row).
// Pure (no React), so it can be script-checked. It is advice only: nothing here changes a choice.
//
// With an API key (a usable platform that needs one, and the key list really loaded) the advice is a premium model; without one the
// Local AI is what is used, and the advice is Qwen2.5-Coder-3B, the best local model Cinqo offers for this. The advice is about the
// choice in effect for that link (its own, else its platform's, else the tool-wide one), so an inherited choice gets the advice for
// what it really uses.
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
  // The short text of the pill.
  text: string
  // What the pill says when it is clicked: why, and what to do.
  info: string
  // The button in the info text: choose = open the AI modal with `suggest` preselected; get-model = go to the Local AI page; none = no button.
  action: 'choose' | 'get-model' | 'none'
  suggest: { platformId: string; model: string } | null
}

const QWEN_NAME = 'Qwen2.5-Coder-3B'

export function crawlAiAdvice(input: {
  usable: Platform[]
  keysKnown: boolean
  choice: CrawlAiChoice
  // linkInstructionFailures of the link.
  failures: number
}): CrawlAiAdvice | null {
  const { usable, keysKnown, choice, failures } = input
  if (!choice.platform) return null
  const premium = keysKnown ? usable.find((p) => p.requiresKey !== false && p.models.length > 0) : undefined
  const effectiveIsLocal = choice.platform.id === LOCAL_PLATFORM_ID
  const usingQwen = effectiveIsLocal && choice.model === RECOMMENDED_LOCAL_CRAWL_MODEL

  if (premium) {
    if (!effectiveIsLocal) {
      return {
        kind: 'ok',
        text: 'Premium AI',
        info: `This link uses a premium model from ${choice.platform.name}.`,
        action: 'none',
        suggest: null,
      }
    }
    // A premium model is advised at once for a link on another local model, but only once the instructions have failed enough times when the
    // best local model is in use. Until then the local advice below applies, and the counter shows how far it is.
    const failedEnough = failures >= PREMIUM_AFTER_FAILURES
    if (failedEnough || !usingQwen) {
      // Only the platform is named, never a model: a platform's first listed model can be a poor one (OpenRouter's is the random free
      // router, which often fails at tool use), so the person picks the model in the dialog.
      return {
        kind: 'premium',
        text: 'Try a premium model',
        info: failedEnough
          ? `The instructions for this link failed ${failures} times with the local model. A premium model from ${premium.name} usually writes them better. Choose it, then pick the model yourself.`
          : `A premium model from ${premium.name} usually writes crawl instructions better than a local one. Choose it, then pick the model yourself.`,
        action: 'choose',
        suggest: { platformId: premium.id, model: '' },
      }
    }
  }

  // No premium platform to offer (or not yet needed): only the Local AI can be advised, and only when it is what this link uses.
  if (!effectiveIsLocal) return null
  if (usingQwen) {
    const counter = failures === 0 ? '' : premium ? ` · ${failures}/${PREMIUM_AFTER_FAILURES} failed` : ` · ${failures} failed`
    return {
      kind: 'ok',
      text: `Best local AI${counter}`,
      info: premium
        ? `${QWEN_NAME} is the best local model for crawl instructions and it is in use. A premium model is suggested here after ${PREMIUM_AFTER_FAILURES} failed attempts; ${failures} failed so far.`
        : `${QWEN_NAME} is the best local model for crawl instructions and it is in use.${failures > 0 ? ` ${failures} attempt${failures === 1 ? '' : 's'} failed so far; an API key (Cinqo's AI menu) would allow a premium model.` : ''}`,
      action: 'none',
      suggest: null,
    }
  }
  const local = usable.find((p) => p.id === LOCAL_PLATFORM_ID)
  if (local?.models.includes(RECOMMENDED_LOCAL_CRAWL_MODEL) === true) {
    return {
      kind: 'qwen',
      text: 'Try Qwen Coder',
      info: `${QWEN_NAME} is the best local model for writing crawl instructions, and it is downloaded. Choose it for this link.`,
      action: 'choose',
      suggest: { platformId: local.id, model: RECOMMENDED_LOCAL_CRAWL_MODEL },
    }
  }
  return {
    kind: 'qwen-missing',
    text: 'Get Qwen Coder',
    info: `${QWEN_NAME} is the best local model for writing crawl instructions, but it is not downloaded yet. Get it in Cinqo's Local AI page.`,
    action: 'get-model',
    suggest: null,
  }
}
