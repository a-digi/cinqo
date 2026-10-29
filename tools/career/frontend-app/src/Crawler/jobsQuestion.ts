// jobsQuestion.ts (step 91) — the "does this page list jobs?" question the
// generation agent asks when its test crawl still extracts 0 items. Pure
// rules, no React. While a question is pending the generation is neither
// running nor failed: it is waiting for the user. See
// plan/ai/tools/career/step-91-ask-user-when-listing-is-empty.md.
import type { JobsQuestion, Portal, PortalLink } from '../api'

// pendingJobsQuestion returns the link's open question, if any.
export function pendingJobsQuestion(link: Pick<PortalLink, 'jobsQuestion'>): JobsQuestion | undefined {
  return link.jobsQuestion
}

// findJobsQuestion looks a link's pending question up in a fresh portals
// payload — used when a generation turn ends, to tell "waiting for the
// user" from "finished without saving".
export function findJobsQuestion(portals: Portal[], linkId: string): JobsQuestion | undefined {
  for (const portal of portals) {
    for (const link of portal.links) {
      if (link.id === linkId) return link.jobsQuestion
    }
  }
  return undefined
}

export const JOBS_QUESTION_TITLE = 'The crawler found no jobs on this page after 2 tries. Does this page list jobs?'

export const JOBS_QUESTION_YES_HINT = 'Yes: the instructions are wrong. The AI is told and tries a different approach.'

export const JOBS_QUESTION_NO_HINT = 'No: nothing is wrong. The link is marked as having no jobs until a crawl finds one.'
