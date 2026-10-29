import type { PortalLink } from '../api'
import { diagnoseEmptyCrawl, emptyCrawlFor } from './emptyCrawl'
import { attemptsLabel, instructionAlertAttempts, isLikelyUncrawlable, uncrawlableTooltip } from './instructionStats'

interface LinkStatusPillsProps {
  link: PortalLink
  // listingGenerating/jobDetailGenerating — a Generate-with-AI run in
  // flight in THIS tab (CrawlPanel's own aiPending states). A run the
  // server already knows about is covered by stats.running; these only
  // close the gap before the next reload. Omitted where the caller has
  // no such state (the link's accordion header).
  listingGenerating?: boolean
  jobDetailGenerating?: boolean
  // showCrawling adds a "Crawling…" pill while a Crawl-now run is active
  // for this link — wanted on a closed link's header, redundant inside
  // CrawlPanel, which has its own run status display.
  showCrawling?: boolean
  className?: string
}

const RED_PILL = 'rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800'

// LinkStatusPills (step 89) — one portal link's at-a-glance state: the
// listing instructions ("Instructed" / "Not instructed"), the step 86/87
// alert (the stronger "probably can't be crawled" pill replaces the
// generic "no instructions after N attempts" one for the listing), and
// optionally an active crawl. The one copy of these pills; previously
// inline in CrawlPanel's header. See
// plan/ai/tools/career/step-89-portal-link-accordion.md.
export function LinkStatusPills({
  link,
  listingGenerating = false,
  jobDetailGenerating = false,
  showCrawling = false,
  className,
}: LinkStatusPillsProps) {
  const hasListing = Boolean(link.crawlInstructions)
  const listingUncrawlable = isLikelyUncrawlable(link.instructionStats.listing, hasListing, listingGenerating)
  const listingAlertAttempts = instructionAlertAttempts(link.instructionStats.listing, hasListing, listingGenerating)
  // Step 90: the latest listing crawl completed but found no jobs.
  const emptyCrawl = emptyCrawlFor(link)
  const jobDetailAlertAttempts = instructionAlertAttempts(
    link.instructionStats.jobDetail,
    Boolean(link.jobDetailCrawlInstructions),
    jobDetailGenerating,
  )

  return (
    <span className={`flex flex-wrap items-center gap-2 ${className ?? ''}`}>
      <span
        className={`rounded-full px-2 py-0.5 text-xs font-medium ${hasListing ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}`}
      >
        {hasListing ? 'Instructed' : 'Not instructed'}
      </span>
      {listingUncrawlable && (
        <span role="status" title={uncrawlableTooltip(link.instructionStats.listing)} className={RED_PILL}>
          Probably can&apos;t be crawled
        </span>
      )}
      {!listingUncrawlable && listingAlertAttempts > 0 && (
        <span role="status" className={RED_PILL}>
          Job list: no instructions after {attemptsLabel(listingAlertAttempts)}
        </span>
      )}
      {jobDetailAlertAttempts > 0 && (
        <span role="status" className={RED_PILL}>
          Job details: no instructions after {attemptsLabel(jobDetailAlertAttempts)}
        </span>
      )}
      {link.jobsQuestion && (
        <span
          role="status"
          title="The AI is waiting for your answer: does this page list jobs?"
          className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800"
        >
          Question for you
        </span>
      )}
      {emptyCrawl && !link.jobsQuestion && (
        <span
          role="status"
          title={diagnoseEmptyCrawl(emptyCrawl).text}
          className="rounded-full bg-orange-100 px-2 py-0.5 text-xs font-medium text-orange-800"
        >
          Crawled 0 jobs
        </span>
      )}
      {showCrawling && link.hasActiveCrawlRun && (
        <span role="status" className="animate-pulse rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
          Crawling…
        </span>
      )}
    </span>
  )
}
