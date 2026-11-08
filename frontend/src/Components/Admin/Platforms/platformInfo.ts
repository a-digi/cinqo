// What the Platforms page's slider tells people about each AI platform. Editorial text, kept here
// (not in the Go registry) because it changes with the providers' offers; the models shown on a
// slide come from the platform API, so they never drift from what Cinqo can really use. No prices
// and no free-credit promises: nothing in the code can verify them, so the copy sends people to the
// provider. Pure (no React), so the rules can be script-checked.

export type Tier = 'free' | 'premium'

export interface PlatformInfo {
  // A short line under the name.
  tagLine: string
  // Which kinds of model the platform offers (drives the badges at the top of the slide).
  tiers: Tier[]
  goodFor: string
  // One more honest sentence about how it is paid for / what to expect.
  note: string
  // Where to get a key: a fixed https page of the provider.
  keyUrl: string
  keyLabel: string
  // The slide's gradient band.
  from: string
  to: string
}

const INFO: Record<string, PlatformInfo> = {
  openai: {
    tagLine: 'GPT models, billed per use',
    tiers: ['premium'],
    goodFor: 'Broad, general-purpose work: writing, analysis, coding and tool use.',
    note: 'You pay OpenAI for what you use, through your own OpenAI account. Check their site for current pricing.',
    keyUrl: 'https://platform.openai.com/api-keys',
    keyLabel: 'Get an OpenAI key',
    from: '#10a37f',
    to: '#0f766e',
  },
  anthropic: {
    tagLine: 'Claude models, billed per use',
    tiers: ['premium'],
    goodFor: 'Careful reasoning, long multi-step tasks and coding.',
    note: 'You pay Anthropic for what you use, through your own Anthropic account. Check their site for current pricing.',
    keyUrl: 'https://platform.claude.com/',
    keyLabel: 'Open the Anthropic console',
    from: '#f59e0b',
    to: '#ea580c',
  },
  openrouter: {
    tagLine: 'One key, many models — including free ones',
    tiers: ['free', 'premium'],
    goodFor: 'Trying Cinqo at no cost, or reaching several providers with a single key.',
    note: "OpenRouter's free router picks a free model for each request. Free models can have limits and change over time; premium models are billed per use. Check OpenRouter for what is available now.",
    keyUrl: 'https://openrouter.ai/keys',
    keyLabel: 'Get an OpenRouter key',
    from: '#6366f1',
    to: '#7c3aed',
  },
}

// A platform this file does not know (one added to the registry later) still gets a slide.
export function infoFor(platformId: string, platformName: string): PlatformInfo {
  return (
    INFO[platformId] ?? {
      tagLine: `AI models from ${platformName}`,
      tiers: ['premium'],
      goodFor: `Conversations answered by ${platformName}'s models.`,
      note: `Get an API key from ${platformName} and add it below. Check their site for what it costs.`,
      keyUrl: '',
      keyLabel: '',
      from: '#475569',
      to: '#1e293b',
    }
  )
}

// Free means OpenRouter's free router, or an id OpenRouter marks as a free variant (":free");
// everything else is billed per use by the provider. A convenience for the slides, not a billing
// guarantee.
export function modelTier(model: string): Tier {
  const id = model.trim().toLowerCase()
  return id === 'openrouter/free' || id.endsWith(':free') ? 'free' : 'premium'
}

export function groupModels(models: string[]): { free: string[]; premium: string[] } {
  const free: string[] = []
  const premium: string[] = []
  for (const m of models) (modelTier(m) === 'free' ? free : premium).push(m)
  return { free, premium }
}

// What a model chip says: the router gets a plain name, anything else is shown as its id.
export function modelDisplay(model: string): string {
  return model.trim().toLowerCase() === 'openrouter/free' ? 'Free models router' : model
}

// "https" links only: a slide never renders a link that is not a fixed provider page.
export function isSafeLink(url: string): boolean {
  try {
    return new URL(url).protocol === 'https:'
  } catch {
    return false
  }
}

// The slides' order: OpenRouter first (it is the one that offers free models, which is what we want people to
// see early), then the others in the order the API returns them.
const FIRST = ['openrouter']

export function orderPlatforms<T extends { id: string }>(platforms: T[]): T[] {
  const rank = (id: string) => {
    const i = FIRST.indexOf(id)
    return i === -1 ? FIRST.length : i
  }
  return platforms
    .map((p, i) => ({ p, i }))
    .sort((a, b) => rank(a.p.id) - rank(b.p.id) || a.i - b.i)
    .map(({ p }) => p)
}
