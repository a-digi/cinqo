// Pure rules of the shared Timeline (plan/ai/tools/career/step-170-onboarding-guide.md, step 1): the step type, the numbers and the
// look of a node, and the check that a list of steps is fit to show. No React, so they can be script-checked.

// Every address a step may link to is a page of this tool.
export const CAREER_PATH_PREFIX = '/tools/career/'

export interface TimelineLink {
  label: string
  // An address inside the tool, e.g. '/tools/career/jobs' (a query string is allowed).
  path: string
}

// Where the person is on the timeline. Nothing writes it yet (the guides show every step the same); a step without a state is neutral.
export type StepState = 'done' | 'current' | 'upcoming'

export interface TimelineStep {
  id: string
  title: string
  text: string
  // A picture for the step; the pins and radial designs show it (a step without one shows its number in the pin).
  icon?: TimelineIcon
  link?: TimelineLink
  tip?: string
  state?: StepState
}

export type NodeLook = 'neutral' | StepState

// The pictures a step may carry (drawn by StepIcon.tsx).
export type TimelineIcon =
  'robot' | 'globe' | 'instructions' | 'pulse' | 'filter' | 'trash' | 'match' | 'building' | 'users' | 'link' | 'eye' | 'briefcase'
export const TIMELINE_ICONS: readonly TimelineIcon[] = [
  'robot',
  'globe',
  'instructions',
  'pulse',
  'filter',
  'trash',
  'match',
  'building',
  'users',
  'link',
  'eye',
  'briefcase',
]

// "1" for the first step: the number in a node is the position, counted from one.
export function numberLabel(index: number): string {
  return String(index + 1)
}

// The last step has no line under it.
export function isLast(index: number, count: number): boolean {
  return index === count - 1
}

export function nodeLook(step: Pick<TimelineStep, 'state'>): NodeLook {
  return step.state ?? 'neutral'
}

// The address without its query string and fragment: what has to be a known page.
export function pathOnly(path: string): string {
  const cut = path.search(/[?#]/)
  return cut === -1 ? path : path.slice(0, cut)
}

const blank = (text: string | undefined) => text === undefined || text.trim() === ''

// Why a list of steps cannot be shown, one message each; an empty list of messages means it is fine.
export function stepProblems(steps: readonly TimelineStep[]): string[] {
  const problems: string[] = []
  if (steps.length === 0) problems.push('there are no steps')
  const seen = new Set<string>()
  steps.forEach((step, index) => {
    const where = `step ${numberLabel(index)}`
    if (blank(step.id)) problems.push(`${where} has no id`)
    else if (seen.has(step.id)) problems.push(`${where} repeats the id "${step.id}"`)
    else seen.add(step.id)
    if (blank(step.title)) problems.push(`${where} has no title`)
    if (blank(step.text)) problems.push(`${where} has no text`)
    if (step.tip !== undefined && blank(step.tip)) problems.push(`${where} has an empty tip`)
    if (step.icon !== undefined && !TIMELINE_ICONS.includes(step.icon)) problems.push(`${where} has an unknown icon "${step.icon}"`)
    if (step.link !== undefined) {
      if (blank(step.link.label)) problems.push(`${where} has a link without a label`)
      if (!step.link.path.startsWith(CAREER_PATH_PREFIX)) problems.push(`${where} links outside the tool: "${step.link.path}"`)
    }
  })
  return problems
}

// The designs of the timeline (plan/ai/tools/career/step-170-onboarding-guide.md, step 8 and its follow-ups). Every guide picks one, so the
// pages do not look alike. zigzag: one narrow framed card, the steps alternating left and right of a centre line with a numbered badge on
// it (one column on a phone). pins: a horizontal bar made of one coloured segment per step with a ring on it for each step, a big arrow at
// the end and a teardrop pin with its text beside it above or below each ring in turn; it scrolls sideways. radial: a ring split into one
// coloured sector per step, a numbered arc band on the outside of each, a white wedge panel with the step's picture and words inside it, and
// a hub with the title in the middle (one column on a screen too narrow for it). cards: two equal columns from a tablet up, the steps take
// turns left and right around a dashed centre line, with the dashboard's soft shapes behind each card (one column on a phone).
export type TimelineDesign = 'zigzag' | 'pins' | 'radial' | 'cards'
export const TIMELINE_DESIGNS: readonly TimelineDesign[] = ['zigzag', 'pins', 'radial', 'cards']

// Which colour of a palette of `size` colours a step gets: they follow each other and start again after the last, so neighbours always
// differ (when there are at least two colours).
export function paletteIndex(index: number, size: number): number {
  return size <= 0 ? 0 : ((index % size) + size) % size
}

// The first step is on the left, the next on the right, and so on.
export function onLeft(index: number): boolean {
  return index % 2 === 0
}

// How a page lays a design out: the zigzag sits in one framed card on the narrow page; the others need the wide page and put their cards
// straight on it.
export function designLayout(design: TimelineDesign): { wide: boolean; framed: boolean } {
  return design === 'zigzag' ? { wide: false, framed: true } : { wide: true, framed: false }
}

// The geometry of the pins design. The timeline scrolls sideways, so it is measured in one unit, the interval between two rings, which is the
// CSS variable --tl-i (it is set once on the list and is smaller on a phone). The first ring sits PIN_FIRST_RING_REM from the left, the rings
// are one interval apart, a step's text block starts half a pin before its ring and is two intervals wide: blocks of neighbours on the same
// side (two steps apart) then touch but never overlap, and the last block ends exactly at the end of the track.
export const PIN_SPACER_REM = 1
export const PIN_HALF_REM = 1.5
export const PIN_FIRST_RING_REM = PIN_SPACER_REM + PIN_HALF_REM
export const PIN_INTERVAL = 'var(--tl-i)'

// Where a step's ring sits, from the left of the track.
export function ringLeft(index: number): string {
  return `calc(${PIN_INTERVAL} * ${String(index)} + ${String(PIN_FIRST_RING_REM)}rem)`
}

// The grid columns of the track: a spacer, then one column per interval, plus one so the last block (two columns wide) fits.
export function trackColumns(count: number): string {
  return `${String(PIN_SPACER_REM)}rem repeat(${String(Math.max(count, 0) + 1)}, ${PIN_INTERVAL})`
}

// The whole width of the track.
export function trackWidth(count: number): string {
  return `calc(${String(PIN_SPACER_REM)}rem + ${PIN_INTERVAL} * ${String(Math.max(count, 0) + 1)})`
}

// The first pin is above the bar, the next below, and so on.
export function isAbove(index: number): boolean {
  return index % 2 === 0
}

// The grid column (counted from one, after the spacer) where a step's text block starts; it spans two columns.
export function blockColumn(index: number): number {
  return index + 2
}
