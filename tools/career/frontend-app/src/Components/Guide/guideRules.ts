// Pure checks of the guides (plan/ai/tools/career/step-170-onboarding-guide.md, step 1): a guide is fit to show when its steps are and
// every page it links to really is a page of the tool (an address of paths.ts), so a guide can never point at a page that is not there.
import * as paths from '../../paths'
import { CAREER_PATH_PREFIX, TIMELINE_DESIGNS, pathOnly, stepProblems } from '../../Shared/Timeline/timelineRules'
import type { Guide } from './guideTypes'

// The pages a guide may link to: every exported address of paths.ts that is one page (a pattern with a ":param" is not).
export const KNOWN_PAGE_PATHS: readonly string[] = (Object.values(paths) as unknown[]).filter(
  (value): value is string => typeof value === 'string' && value.startsWith(CAREER_PATH_PREFIX) && !value.includes(':'),
)

const blank = (text: string) => text.trim() === ''

// Why a guide cannot be shown, one message each (empty = fine). knownPaths is passed in so the check can be run on its own.
export function guideProblems(guide: Guide, knownPaths: readonly string[] = KNOWN_PAGE_PATHS): string[] {
  const problems: string[] = []
  const name = blank(guide.id) ? '(no id)' : guide.id
  if (blank(guide.id)) problems.push('a guide has no id')
  if (blank(guide.title)) problems.push(`guide ${name} has no title`)
  if (blank(guide.description)) problems.push(`guide ${name} has no description`)
  if (!TIMELINE_DESIGNS.includes(guide.design)) problems.push(`guide ${name} has an unknown design "${guide.design}"`)
  for (const problem of stepProblems(guide.steps)) problems.push(`guide ${name}: ${problem}`)
  guide.steps.forEach((step) => {
    if (step.link !== undefined && step.link.path.startsWith(CAREER_PATH_PREFIX) && !knownPaths.includes(pathOnly(step.link.path))) {
      problems.push(`guide ${name}: step "${step.id}" links to an unknown page "${step.link.path}"`)
    }
  })
  return problems
}

// Every guide must be fine and the ids must differ from guide to guide.
export function guidesProblems(guides: readonly Guide[], knownPaths: readonly string[] = KNOWN_PAGE_PATHS): string[] {
  const problems: string[] = []
  const seen = new Set<string>()
  for (const guide of guides) {
    if (seen.has(guide.id)) problems.push(`two guides have the id "${guide.id}"`)
    seen.add(guide.id)
    problems.push(...guideProblems(guide, knownPaths))
  }
  return problems
}
