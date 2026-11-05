import { useEffect, useState } from 'react'
import type { PortalLink } from '../api'
import { fetchLinkCrawlAvailability, showLinkCrawl, type LinkCrawlAvailability } from './linkCrawl'
import { LinkCrawlPanel } from './LinkCrawl/LinkCrawlPanel'

// The link crawler's place on a portal link (plan 185). The panel appears ONLY while the backend says the feature is available for
// this link (Katana is the selected Browser engine and the link needs no login); in every other case, and while the answer is
// unknown, this renders nothing at all. The answer is asked again when the tab becomes visible, so switching the engine on the Browser
// & Engine page shows or hides the panel without a reload. Everything the panel does is enforced again by the backend.
export function LinkCrawlGate({ link, onReload }: { link: PortalLink; onReload: () => void }) {
  const [answer, setAnswer] = useState<LinkCrawlAvailability | null>(null)
  const linkId = link.id

  useEffect(() => {
    let cancelled = false
    let controller: AbortController | null = null
    const ask = () => {
      controller?.abort()
      controller = new AbortController()
      fetchLinkCrawlAvailability(linkId, controller.signal)
        .then((a) => {
          if (!cancelled) setAnswer(a)
        })
        .catch(() => {
          // Could not be read: fail closed, hide the feature.
          if (!cancelled && !controller?.signal.aborted) setAnswer(null)
        })
    }
    const onVisible = () => {
      if (document.visibilityState === 'visible') ask()
    }
    ask()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      cancelled = true
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [linkId])

  if (!showLinkCrawl(answer)) return null
  return <LinkCrawlPanel link={link} onReload={onReload} />
}
