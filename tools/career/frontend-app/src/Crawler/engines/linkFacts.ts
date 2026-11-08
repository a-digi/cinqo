// Small neutral helpers the engines share (no engine knowledge).
import type { LinkFacts } from './types'

export const hasText = (text: string | null | undefined): boolean => (text ?? '').trim() !== ''

export const hasRule = (link: LinkFacts): boolean => (link.linkRules ?? null) !== null
export const hasAiInstructions = (link: LinkFacts): boolean => hasText(link.crawlInstructions)
