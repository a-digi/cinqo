import { useEffect, useState } from 'react'
import type { PortalLink } from '../api'
import { fetchLinkCrawlAvailability, saveLinkRules, showLinkCrawl, type LinkCrawlAvailability } from './linkCrawl'
import { LinkCrawlPanel } from './LinkCrawl/LinkCrawlPanel'

// The link crawler's place on a portal link (plan 185). The panel appears ONLY while the backend says the feature is available for
// this link (Katana is the selected Browser engine and the link needs no login); in every other case, and while the answer is
// unknown, nothing of the feature is shown — with one exception: a link that is set to the link crawler while it is NOT available gets a
// short notice and a way back to its AI instructions, because its crawls stop with a message until Katana is selected again and a
// person must never be stuck on a method they cannot see. The answer is asked again when the tab becomes visible, so switching the
// engine on the Browser & Engine page shows or hides the panel without a reload. Everything the panel does is enforced again by the backend.
export function LinkCrawlGate({ link, onReload }: { link: PortalLink; onReload: () => void }) {
  const [answer, setAnswer] = useState<LinkCrawlAvailability | null>(null)
  const [asked, setAsked] = useState(false)
  const linkId = link.id

  useEffect(() => {
    let cancelled = false
    let controller: AbortController | null = null
    const ask = () => {
      controller?.abort()
      controller = new AbortController()
      fetchLinkCrawlAvailability(linkId, controller.signal)
        .then((a) => {
          if (cancelled) return
          setAnswer(a)
          setAsked(true)
        })
        .catch(() => {
          // Could not be read: fail closed, hide the feature.
          if (!cancelled && !controller?.signal.aborted) {
            setAnswer(null)
            setAsked(true)
          }
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

  if (showLinkCrawl(answer)) return <LinkCrawlPanel link={link} onReload={onReload} />
  if (asked && link.crawlMethod === 'links') return <LinkCrawlSwitchBack link={link} onReload={onReload} reason={answer?.message ?? ''} />
  return null
}

// A link on the link crawler method while the feature is not available: say so, and offer the way back.
function LinkCrawlSwitchBack({ link, onReload, reason }: { link: PortalLink; onReload: () => void; reason: string }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  function back() {
    setBusy(true)
    setError('')
    saveLinkRules(link.id, 'instructions')
      .then(onReload)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Something went wrong.')
      })
      .finally(() => {
        setBusy(false)
      })
  }
  return (
    <div className="mb-2 rounded-2xl border border-amber-300 bg-amber-50 p-3 text-xs text-amber-900" data-testid="link-crawl-switch-back">
      <span className="font-semibold">Link crawler</span> — this link is set to the link crawler, but it is not available right now
      {reason ? `: ${reason}` : '.'} Crawls of this link stop with a message until then; they never fall back to the AI instructions.{' '}
      <button
        type="button"
        disabled={busy}
        onClick={back}
        className="rounded-md border border-amber-400 bg-white px-2.5 py-1 font-medium hover:bg-amber-100 disabled:opacity-50"
      >
        Crawl this link with its AI instructions again
      </button>
      {error && (
        <p role="alert" className="mt-1 text-red-700">
          {error}
        </p>
      )}
    </div>
  )
}
