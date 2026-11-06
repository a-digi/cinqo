// autoDiscoveryVisibility.ts (step 88) — when auto-discovery controls are
// worth showing. Auto-discovery crawls a link's LISTING page with what the
// Browser's selected engine needs (a rule under Katana, AI instructions
// otherwise; plan 190), and the backend already refuses to crawl a link
// without it (ListPortalLinksEligibleForAutoDiscovery), so a
// control for such a link, or a global switch with no link it could ever
// act on, only suggests something that cannot happen. This is display
// logic only; no stored setting is ever changed by hiding a control. See
// plan/ai/tools/career/step-88-auto-discovery-only-for-instructed-links.md.
import type { Portal, PortalLink } from '../api'
import { hasListingFor, type EngineReading } from './crawlEngine'

// canCrawlListing (plan 185, plan 190): the link has what the Browser's selected engine needs to crawl its listing — a link-crawler rule
// under Katana, the AI instructions under Rod (and for a link that needs a login). The same condition the backend's auto-discovery uses.
export function canCrawlListing(
  link: Pick<PortalLink, 'crawlInstructions' | 'linkRules'> & { requiresLogin?: boolean; loginSession?: string },
  engine: EngineReading = null,
): boolean {
  // Auto-discovery of a link that needs a login stays manual-only (plan 195): its kept login is not looked at here.
  return hasListingFor({ ...link, loginSession: undefined }, engine)
}

// anyLinkAutoDiscoverable: at least one link, across every portal, that
// the global auto-discovery could crawl.
export function anyLinkAutoDiscoverable(portals: Pick<Portal, 'links'>[], engine: EngineReading = null): boolean {
  return portals.some((portal) => portal.links.some((link) => canCrawlListing(link, engine)))
}
