// Which AI platform and model a portal link uses for crawling and for preparing its
// crawl instructions, as one pure rule (step 102): the link's own choice, else its
// portal's, else the tool-wide one (aiChoice.ts, the Settings page). Pure (no React,
// no fetching) so it can be script-checked; the Portals page feeds it.
import { resolveAiChoice, type AiChoice, type SavedChoice } from './aiChoice'
import type { Platform } from './platformRepository'

// One level's own choice. Both empty means "none set". The fields are optional on
// purpose: a record that arrives without them (an older backend) must read as "none".
export interface AiOverride {
  platformId?: string | null
  model?: string | null
}

export type CrawlAiSource = 'link' | 'portal' | 'global'

// A level whose platform no longer exists in core: skipped, and reported so the page can say so.
export interface StaleOverride {
  level: 'link' | 'portal'
  platformId: string
}

export interface CrawlAiChoice {
  platform: Platform | null
  // The model to use, or null when the platform lists none.
  model: string | null
  // Which level the platform and model come from.
  source: CrawlAiSource
  // Levels that had a choice set but were skipped because their platform is gone.
  stale: StaleOverride[]
}

function platformIdOf(o: AiOverride | null | undefined): string {
  return (o?.platformId ?? '').trim()
}

// The model for a platform: the one asked for if the platform still offers it, else the platform's first.
function modelFor(platform: Platform, wanted: string | null | undefined): string | null {
  const model = (wanted ?? '').trim()
  if (model !== '' && platform.models.includes(model)) return model
  return platform.models.length > 0 ? platform.models[0] : null
}

export function resolveCrawlAi(input: {
  link?: AiOverride | null
  portal?: AiOverride | null
  platforms: Platform[]
  // Ids of platforms that have at least one API key (for the tool-wide automatic pick).
  keyedPlatformIds: string[]
  saved: SavedChoice
}): CrawlAiChoice {
  const stale: StaleOverride[] = []
  const levels: { level: 'link' | 'portal'; override: AiOverride | null | undefined }[] = [
    { level: 'link', override: input.link },
    { level: 'portal', override: input.portal },
  ]
  for (const { level, override } of levels) {
    const platformId = platformIdOf(override)
    if (platformId === '') continue
    const platform = input.platforms.find((p) => p.id === platformId)
    if (!platform) {
      stale.push({ level, platformId })
      continue
    }
    return { platform, model: modelFor(platform, override?.model), source: level, stale }
  }
  const global: AiChoice = resolveAiChoice(input.platforms, input.keyedPlatformIds, input.saved)
  return { platform: global.platform, model: global.model, source: 'global', stale }
}
