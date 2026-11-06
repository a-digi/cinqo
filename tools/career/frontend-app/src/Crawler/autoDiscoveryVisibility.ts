// autoDiscoveryVisibility.ts (step 88) — when auto-discovery controls are
// worth showing. Auto-discovery crawls a link's LISTING page using its
// stored crawl_instructions, and the backend already refuses to crawl a
// link without them (ListPortalLinksEligibleForAutoDiscovery), so a
// control for such a link, or a global switch with no link it could ever
// act on, only suggests something that cannot happen. This is display
// logic only; no stored setting is ever changed by hiding a control. See
// plan/ai/tools/career/step-88-auto-discovery-only-for-instructed-links.md.
import type { Portal, PortalLink } from '../api'
import { hasListingFor, type EngineReading } from './crawlEngine'

// hasListingInstructions: the link has listing crawl instructions. Those
// can only be saved after passing the backend's own validation, so a
// non-blank value already means "valid" in the only sense the tool can
// check. Job-detail instructions are not required — auto-discovery never
// reads them.
export function hasListingInstructions(link: Pick<PortalLink, 'crawlInstructions'>): boolean {
  return (link.crawlInstructions ?? '').trim() !== ''
}

// canCrawlListing (plan 185, plan 190): the link has what the Browser's selected engine needs to crawl its listing — a link-crawler rule
// under Katana, the AI instructions under Rod (and for a link that needs a login). The same condition the backend's auto-discovery uses.
export function canCrawlListing(
  link: Pick<PortalLink, 'crawlInstructions' | 'linkRules'> & { requiresLogin?: boolean },
  engine: EngineReading = null,
): boolean {
  return hasListingFor(link, engine)
}

// anyLinkAutoDiscoverable: at least one link, across every portal, that
// the global auto-discovery could crawl.
export function anyLinkAutoDiscoverable(portals: Pick<Portal, 'links'>[], engine: EngineReading = null): boolean {
  return portals.some((portal) => portal.links.some((link) => canCrawlListing(link, engine)))
}
