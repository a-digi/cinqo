import { fetchPortals, finishInstructionAttempt, updatePortalLink } from '../api'
import { CoreApiError } from '../Cinqo/Http/client'
import { awaitTurnCompletion } from '../Cinqo/Conversation/conversation'
import { fetchCrawlEngine, hasListingFor } from './crawlEngine'
import { startCrawlJobDetailsNow, startCrawlNow } from './crawlNow'
import { createRunFollower } from './runFollower'

// The one run follower of this tab, wired to the real API (see runFollower.ts). main.tsx starts it when the bundle loads.
export const runFollower = createRunFollower({
  fetchLinks: () => fetchPortals().then((portals) => portals.flatMap((portal) => portal.links)),
  readEngine: () => fetchCrawlEngine(),
  hasListing: (link, engine) => hasListingFor(link, engine),
  updateLink: (linkId, patch) => updatePortalLink(linkId, patch),
  startCrawl: (linkId, kind) => (kind === 'listing' ? startCrawlNow(linkId) : startCrawlJobDetailsNow(linkId)),
  awaitTurn: awaitTurnCompletion,
  recordOutcome: (linkId, kind, errorText) =>
    finishInstructionAttempt(linkId, kind, errorText).catch((err: unknown) => {
      console.error('failed to record instruction attempt finish for portal link', linkId, err)
      return undefined
    }),
  errorText: (err, kind) =>
    err instanceof CoreApiError && (err.status === 401 || err.status === 403)
      ? 'Ask an admin to grant you access to AI conversations.'
      : err instanceof Error
        ? err.message
        : kind === 'listing'
          ? 'Failed to generate crawl instructions.'
          : 'Failed to generate job detail crawl instructions.',
  onChange: () => undefined,
})
