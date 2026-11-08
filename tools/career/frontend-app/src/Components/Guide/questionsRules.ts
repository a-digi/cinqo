// Pure rules of the Questions page (plan/ai/tools/career/step-170-onboarding-guide.md, step 6): which question is open and why a list of
// questions is not fit to show. No React, so they can be script-checked.
import { CAREER_PATH_PREFIX, pathOnly } from '../../Shared/Timeline/timelineRules'
import { KNOWN_PAGE_PATHS } from './guideRules'
import type { Question } from './guideTypes'

// One answer is open at a time: a click opens a closed question (closing the open one) and closes the open one.
export function toggleOpen(current: string | null, clicked: string): string | null {
  return current === clicked ? null : clicked
}

const blank = (text: string) => text.trim() === ''

// Why the questions cannot be shown, one message each (empty = fine). Every link must be a page of paths.ts.
export function questionsProblems(questions: readonly Question[], knownPaths: readonly string[] = KNOWN_PAGE_PATHS): string[] {
  const problems: string[] = []
  if (questions.length === 0) problems.push('there are no questions')
  const seen = new Set<string>()
  questions.forEach((q, index) => {
    const where = `question ${String(index + 1)}`
    if (blank(q.id)) problems.push(`${where} has no id`)
    else if (seen.has(q.id)) problems.push(`${where} repeats the id "${q.id}"`)
    else seen.add(q.id)
    if (blank(q.question)) problems.push(`${where} has no question`)
    if (blank(q.answer)) problems.push(`${where} has no answer`)
    if (q.link !== undefined) {
      if (blank(q.link.label)) problems.push(`${where} has a link without a label`)
      if (!q.link.path.startsWith(CAREER_PATH_PREFIX)) problems.push(`${where} links outside the tool: "${q.link.path}"`)
      else if (!knownPaths.includes(pathOnly(q.link.path))) problems.push(`${where} links to an unknown page "${q.link.path}"`)
    }
  })
  return problems
}
