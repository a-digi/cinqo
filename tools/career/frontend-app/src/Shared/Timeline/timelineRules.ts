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
  link?: TimelineLink
  tip?: string
  state?: StepState
}

export type NodeLook = 'neutral' | StepState

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
    if (step.link !== undefined) {
      if (blank(step.link.label)) problems.push(`${where} has a link without a label`)
      if (!step.link.path.startsWith(CAREER_PATH_PREFIX)) problems.push(`${where} links outside the tool: "${step.link.path}"`)
    }
  })
  return problems
}
