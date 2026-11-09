import type { Motif } from './PlatformSlides'
import type { Tier } from './platformInfo'

// The look of the platform slides, shared with the How To landing pages (plan 219): the badge of a model tier and the line art of each platform.

export const TIER_BADGE: Record<Tier, { label: string; className: string }> = {
  free: { label: 'Free models', className: 'bg-emerald-400/20 text-emerald-100 ring-emerald-300/40' },
  premium: { label: 'Premium models', className: 'bg-violet-400/20 text-violet-100 ring-violet-300/40' },
}

export const MOTIFS: Record<string, Motif> = { openai: 'rings', anthropic: 'burst', openrouter: 'network' }
