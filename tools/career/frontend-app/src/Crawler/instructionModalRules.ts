// What the header of an instruction modal says about the link being edited (plan/ai/tools/career/step-246-instruction-modal-two-columns-and-design.md).
// Pure: no React, so it can be script-checked.
export type InstructionKind = 'rules' | 'crawl' | 'jobDetail'

export interface InstructionContext {
  url: string
  engine: string | null
  state: { text: string; tone: 'saved' | 'empty' }
}

export interface InstructionLinkFacts {
  url: string
  linkRules: unknown
  crawlInstructions?: string | null
  jobDetailCrawlInstructions?: string | null
}

const ENGINE_NAMES: Record<string, string> = { katana: 'Katana', rod: 'Rod' }

// The engine as a person knows it, or null while it is not known (nothing is said then).
export function engineName(engine: string | null): string | null {
  if (engine === null || engine === 'unknown') return null
  return ENGINE_NAMES[engine] ?? engine
}

const filled = (text: string | null | undefined): boolean => text !== undefined && text !== null && text.trim() !== ''

// The context line of the modal for one kind: the link's address, its engine and whether what the modal edits is saved.
export function instructionContext(kind: InstructionKind, link: InstructionLinkFacts, engine: string | null): InstructionContext {
  const saved =
    kind === 'rules'
      ? link.linkRules !== null && link.linkRules !== undefined
      : kind === 'crawl'
        ? filled(link.crawlInstructions)
        : filled(link.jobDetailCrawlInstructions)
  const noun = kind === 'rules' ? 'Rule' : 'Instructions'
  return {
    url: link.url,
    engine: engineName(engine),
    state: saved ? { text: `${noun} saved`, tone: 'saved' } : { text: 'Not set yet', tone: 'empty' },
  }
}
