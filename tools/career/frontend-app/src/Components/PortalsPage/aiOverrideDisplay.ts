// What the AI chip on a portal or link says about its own override (step 102). Pure
// (no React), so it can be script-checked. The fields of an override are optional
// because a record can arrive without them; that reads as "none set".
import type { AiOverride, CrawlAiChoice } from '../../Cinqo/Platform/crawlAiChoice'
import type { Platform } from '../../Cinqo/Platform/platformRepository'

export interface AiChip {
  text: string
  // default: the tool-wide choice; inherited: the portal's choice, not this link's own;
  // set: a choice of its own that works; stale: a choice whose platform is gone.
  kind: 'default' | 'inherited' | 'set' | 'stale'
}

export function aiChip(override: AiOverride | null | undefined, platforms: Platform[]): AiChip {
  const platformId = (override?.platformId ?? '').trim()
  if (platformId === '') return { text: 'AI: Default', kind: 'default' }
  const platform = platforms.find((p) => p.id === platformId)
  if (!platform) return { text: 'AI: platform not available', kind: 'stale' }
  const model = (override?.model ?? '').trim()
  return { text: model === '' ? `AI: ${platform.name}` : `AI: ${platform.name} · ${model}`, kind: 'set' }
}

// "OpenRouter · model" for a resolved choice, or a plain sentence when there is none.
export function choiceText(platform: Platform | null, model: string | null): string {
  if (!platform) return 'no AI platform is available'
  return model === null || model === '' ? platform.name : `${platform.name} · ${model}`
}

// A portal or link as the backend sends it carries its choice as aiPlatformId and aiModel;
// the resolver and the modal speak platformId and model.
export function overrideOf(record: { aiPlatformId?: string | null; aiModel?: string | null }): AiOverride {
  return { platformId: record.aiPlatformId ?? '', model: record.aiModel ?? '' }
}

const SOURCE_TEXT = {
  link: 'this link',
  portal: "this portal's choice",
  global: 'the tool-wide choice from Settings',
} as const

// A resolved choice with where it comes from, e.g. "OpenRouter · model (the tool-wide choice from Settings)".
export function describeChoice(choice: Pick<CrawlAiChoice, 'platform' | 'model' | 'source'>): string {
  return `${choiceText(choice.platform, choice.model)} (${SOURCE_TEXT[choice.source]})`
}

// What a link's chip says: its own choice, the portal's it inherits, the tool-wide default,
// or a warning when its own platform is gone (it is then skipped, but still needs attention).
export function linkChip(choice: CrawlAiChoice): AiChip {
  if (choice.stale.some((s) => s.level === 'link')) return { text: 'AI: platform not available', kind: 'stale' }
  if (choice.source === 'link') return { text: `AI: ${choiceText(choice.platform, choice.model)} (this link)`, kind: 'set' }
  if (choice.source === 'portal') return { text: `AI: ${choiceText(choice.platform, choice.model)} (portal)`, kind: 'inherited' }
  return { text: 'AI: Default', kind: 'default' }
}
