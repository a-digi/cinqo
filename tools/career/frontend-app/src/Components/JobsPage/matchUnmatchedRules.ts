// "Match unmatched jobs" (step 134): the pure rule that words the result of a press, kept apart from the page so it can
// be checked on its own.
import type { MatchUnmatchedResult } from '../../api'

function plural(n: number): string {
  return `${String(n)} ${n === 1 ? 'job' : 'jobs'}`
}

export function matchUnmatchedMessage(r: MatchUnmatchedResult): string {
  if (r.unmatched === 0) return 'Every job already has a match.'
  const parts: string[] = []
  if (r.matched > 0) parts.push(`Matched ${plural(r.matched)}`)
  if (r.skippedExisting > 0) parts.push(`${plural(r.skippedExisting)} got a match in the meantime`)
  if (r.unjudged > 0)
    parts.push(`${plural(r.unjudged)} could not be judged yet: give a target role skills or desired titles and it will be matched`)
  if (r.failed > 0) parts.push(`${plural(r.failed)} could not be matched because of an error`)
  let text = `${parts.join('. ')}.`
  if (r.more) text += ' More jobs are waiting: press the button again.'
  return text
}
