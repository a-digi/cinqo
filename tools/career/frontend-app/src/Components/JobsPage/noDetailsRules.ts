// noDetailsRules.ts (plan 214) — what the info icon next to the title of a job whose details were not crawled says. It promises "soon" only where a crawl
// is really running, queued or scheduled; otherwise it says why nothing is coming and what the person can do. Pure, so it can be script-checked.
import type { MissingDetailState } from '../../api'

export function noDetailsNote(state: MissingDetailState | undefined, platformName: string | undefined): string {
  const platform = (platformName ?? '').trim() !== '' ? (platformName ?? '').trim() : 'its platform'
  switch (state) {
    case 'running':
      return 'The details of this job are being crawled right now. They appear here in a moment.'
    case 'queued':
      return 'This job is waiting in the crawl queue. Its details will be crawled soon.'
    case 'scheduled':
      return `No details yet. The job-detail crawl of ${platform} is next, so they should appear soon.`
    case 'ready':
      return 'No details crawled yet, and no crawl is scheduled. Use “Crawl missing details” to get them now.'
    case 'noInstructions':
      return `No details yet: ${platform} has no job detail instructions. They are written first (see Platforms).`
    case 'needsLogin':
      return `No details yet: ${platform} needs a login. Crawl this job on the platform's link.`
    case 'noPortal':
      return 'No details: this job has no platform or posting address to crawl from.'
    default:
      return 'No details crawled yet.'
  }
}
