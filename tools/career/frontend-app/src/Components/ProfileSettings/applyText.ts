// The words of the "apply to existing jobs" panel (step 143): what a preview or a finished run says, kept apart from the page so
// it can be checked on its own. The numbers come from the server's summary.
import type { ApplyRulesSummary } from '../../api'
import { plural } from '../CompaniesPage/companyStats'

// "8 to To check, 4 to Top pick": where the added jobs go, in the order the server gives them.
export function statusParts(s: ApplyRulesSummary): string {
  return s.statuses.map((c) => `${String(c.count)} to ${c.statusName}`).join(', ')
}

// The first line of a preview ("would be added") or of a finished run ("added").
export function headline(s: ApplyRulesSummary, dryRun: boolean): string {
  if (s.total === 0) return dryRun ? 'No job would be added.' : 'No job was added.'
  const verb = dryRun ? 'would be' : s.total === 1 ? 'was' : 'were'
  return `${plural(s.total, 'job', 'jobs')} ${verb} added: ${statusParts(s)}.`
}

// The rest: how many jobs were looked at and what became of the ones that were not added; nothing when no job exists.
export function details(s: ApplyRulesSummary): string {
  if (s.jobs === 0) return 'There are no jobs yet.'
  const parts = [`${plural(s.jobs, 'job', 'jobs')} checked`]
  if (s.onBoard > 0) parts.push(`${String(s.onBoard)} already on the board (left as they are)`)
  if (s.below > 0) parts.push(`${String(s.below)} below your rules`)
  if (s.notJudged > 0) parts.push(`${String(s.notJudged)} could not be judged (this profile has nothing to compare for them)`)
  return `${parts.join('; ')}.`
}

// A failed run: the reason, and what was added before it when anything was (that stays on the board).
export function failureText(message: string, partial: ApplyRulesSummary | null): string {
  if (partial === null || partial.total === 0) return message
  return `${message} ${plural(partial.total, 'job', 'jobs')} ${partial.total === 1 ? 'was' : 'were'} added before it stopped and stay${partial.total === 1 ? 's' : ''} on the board.`
}
