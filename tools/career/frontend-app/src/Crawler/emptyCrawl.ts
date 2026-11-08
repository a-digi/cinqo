// emptyCrawl.ts (step 90) — when a link's latest listing crawl found no
// jobs, whether to flag it, and what is known about why. The crawler can
// only be certain in one case (items matched but none usable); when
// nothing matched at all, "the page has no jobs" and "the container
// selector is wrong" look identical to it, so the diagnosis lays out the
// evidence and lets the user decide. Pure rules, no React. See
// plan/ai/tools/career/step-90-zero-jobs-crawl-flag.md.
import type { LastListingCrawl, PortalLink } from '../api'

export type EmptyCrawlKind =
  // Items matched but none had a title and URL: the instructions are wrong.
  | 'items-unusable'
  // Fields matched nothing (flat mode): the selectors are probably wrong.
  | 'selectors-no-match'
  // The link produced jobs before: the page changed or is temporarily empty.
  | 'used-to-work'
  // The link never produced a job: can't tell, check the page.
  | 'never-had-jobs'

export interface EmptyCrawlDiagnosis {
  kind: EmptyCrawlKind
  // conclusive is true only when the crawl itself proves the instructions
  // don't fit the page, so "mark as expected" makes no sense.
  conclusive: boolean
  text: string
}

export function foundNoJobs(c: LastListingCrawl): boolean {
  return c.jobsSaved + c.jobsUpdated === 0
}

// isAcknowledged: the user marked the current zero streak as expected
// and no crawl has found a job since. An acknowledgement older than the
// last run that found jobs belongs to a streak that has ended.
export function isAcknowledged(c: LastListingCrawl): boolean {
  if (!c.acknowledgedAt) return false
  if (!c.lastJobsAt) return true
  return Date.parse(c.acknowledgedAt) > Date.parse(c.lastJobsAt)
}

// emptyCrawlFor returns the summary to flag, or undefined. Flagged only
// while the link still has listing instructions, no crawl is running right
// now, the latest completed crawl found no jobs, and the user has not
// marked it as expected.
export function emptyCrawlFor(link: PortalLink): LastListingCrawl | undefined {
  const c = link.lastListingCrawl
  if (!c || !link.crawlInstructions || link.hasActiveCrawlRun) return undefined
  if (!foundNoJobs(c) || isAcknowledged(c)) return undefined
  return c
}

function when(iso: string | undefined): string {
  return iso ? new Date(iso).toLocaleString() : 'unknown'
}

// diagnoseEmptyCrawl: first match wins.
export function diagnoseEmptyCrawl(c: LastListingCrawl): EmptyCrawlDiagnosis {
  if (c.jobsSkipped > 0) {
    return {
      kind: 'items-unusable',
      conclusive: true,
      text:
        `${c.jobsSkipped} item${c.jobsSkipped === 1 ? '' : 's'} matched on the page, but none had both a title and a URL. ` +
        `The instructions don't match this page's markup. Regenerate them.`,
    }
  }
  if (c.notFoundFields.length > 0) {
    return {
      kind: 'selectors-no-match',
      conclusive: false,
      text:
        `These fields matched nothing on the page: ${c.notFoundFields.join(', ')}. ` +
        `The selectors are probably wrong, unless the page really has no jobs right now. Open the page to check.`,
    }
  }
  if (c.jobsFromThisLink > 0) {
    return {
      kind: 'used-to-work',
      conclusive: false,
      text:
        `This link returned ${c.jobsFromThisLink} job${c.jobsFromThisLink === 1 ? '' : 's'} before` +
        (c.lastJobsAt ? ` (last crawl with jobs: ${when(c.lastJobsAt)})` : '') +
        ` and none now. The page may have changed, or may be temporarily empty. Open the page to check.`,
    }
  }
  return {
    kind: 'never-had-jobs',
    conclusive: false,
    text:
      `This link has never returned a job. Open the page: if it lists jobs, the instructions are wrong; ` +
      `if it really has none, nothing is wrong and you can mark it as expected.`,
  }
}

// emptyCrawlFacts is the one-line "what happened" shown with the diagnosis.
export function emptyCrawlFacts(c: LastListingCrawl): string {
  const who = c.triggeredBy === 'auto_discovery' ? 'automatic' : 'manual'
  const pages = `${c.pagesVisited} page${c.pagesVisited === 1 ? '' : 's'}`
  const streak = c.zeroStreak > 1 ? ` · ${c.zeroStreak} crawls in a row without jobs` : ''
  return `Last crawl: ${when(c.finishedAt)} (${who}) · ${pages} visited${streak}`
}
