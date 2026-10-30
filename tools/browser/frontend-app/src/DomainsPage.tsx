import { useCallback, useEffect, useState } from 'react'
import { cancelDomainRequest, clearDomainNow, fetchDomains, forgetDomain, type DomainEntry } from './api'
import { ClearanceBar } from './ClearanceBar'
import { summarizeDomains } from './domainRules'
import { GlobeIcon } from './icons'
import { PageHeaderCard } from './Shared/PageHeaderCard'
import { inputClass } from './Shared/inputClass'
import { plural } from './Shared/plural'
import { PAGE_ACCENT, accentFor, initialOf } from './Shared/theme'

// Domains page (step 96) — the sites that asked for a Cloudflare check. For each:
// whether a person's clearance is kept (and until when) and whether a Chrome window
// is open for someone right now. "Clear now" opens that window on purpose, before an
// agent needs it; the person clears it there, and the clearance is kept until it
// expires so agents and auto-discovery get through. The tool never solves a check
// itself. A clearance bar above the list is the overview (cleared / expired / not
// cleared, windows open, next expiry). See
// plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.

const POLL_MS = 3000

function formatTime(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

export function DomainsPage() {
  const [domains, setDomains] = useState<DomainEntry[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('') // the domain an action is running for
  const [newDomain, setNewDomain] = useState('')

  const load = useCallback(() => {
    fetchDomains()
      .then((list) => {
        setDomains(list)
        setError('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // Keep the list fresh while a window is open for someone.
  const anyWaiting = domains?.some((d) => d.request?.state === 'waiting') ?? false
  useEffect(() => {
    if (!anyWaiting) return
    const timer = window.setInterval(load, POLL_MS)
    return () => {
      window.clearInterval(timer)
    }
  }, [anyWaiting, load])

  function run(domain: string, action: () => Promise<void>) {
    setBusy(domain)
    setError('')
    action()
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setBusy('')
      })
  }

  function addAndClear() {
    const value = newDomain.trim()
    if (!value) return
    run(value, () => clearDomainNow(value))
    setNewDomain('')
  }

  // "Next expires in" needs a clock: re-read it each render (the list polls while a window is open).
  const now = new Date()
  const summary = summarizeDomains(domains ?? [], now)

  return (
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<GlobeIcon />}
        title="Domains"
        description="Sites that asked for a check only a person can pass. Clear now opens a Chrome window on this computer: you clear the check there and the clearance is kept until it expires. The tool never solves the check itself."
        pill={domains ? { text: plural(domains.length, 'domain', 'domains') } : undefined}
      />

      <ClearanceBar summary={summary} now={now} />

      {error && <div className="mb-3 rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}

      <div
        className="mb-5 flex flex-wrap items-center gap-2 rounded-3xl border p-3"
        style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
      >
        <label htmlFor="cinqo-browser-new-domain" className="text-sm font-medium">
          Clear a site now
        </label>
        <input
          id="cinqo-browser-new-domain"
          type="text"
          value={newDomain}
          placeholder="example.com"
          onChange={(e) => {
            setNewDomain(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') addAndClear()
          }}
          className={`min-w-[14rem] flex-1 ${inputClass}`}
        />
        <button
          type="button"
          onClick={addAndClear}
          disabled={!newDomain.trim() || busy !== ''}
          className="rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-900 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Clear now
        </button>
      </div>

      {domains === null && !error && <p className="text-sm text-gray-400">Loading…</p>}
      {domains?.length === 0 && (
        <div
          className="rounded-3xl border p-8 text-center text-sm text-gray-500"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          No site has asked for a check yet. When one does, it appears here — or add one above to clear it ahead of time.
        </div>
      )}

      <div className="space-y-3">
        {domains?.map((d) => (
          <DomainRow
            key={d.domain}
            entry={d}
            busy={busy === d.domain}
            onClear={() => {
              run(d.domain, () => clearDomainNow(d.domain))
            }}
            onCancel={(requestId) => {
              run(d.domain, () => cancelDomainRequest(requestId))
            }}
            onForget={() => {
              if (window.confirm(`Forget ${d.domain}? Its kept clearance is removed and the next check will need a person again.`)) {
                run(d.domain, () => forgetDomain(d.domain))
              }
            }}
          />
        ))}
      </div>
    </div>
  )
}

function DomainRow({
  entry,
  busy,
  onClear,
  onCancel,
  onForget,
}: {
  entry: DomainEntry
  busy: boolean
  onClear: () => void
  onCancel: (requestId: string) => void
  onForget: () => void
}) {
  const waiting = entry.request?.state === 'waiting'
  return (
    <div
      className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
      style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span
            className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
            style={{ background: accentFor(entry.domain).to }}
            aria-hidden="true"
          >
            {initialOf(entry.domain)}
          </span>
          <div className="min-w-0">
            <div className="break-all text-sm font-semibold">{entry.domain}</div>
            {entry.reason && <div className="text-xs text-gray-500">Seen: {entry.reason}</div>}
            {entry.firstDetectedAt && <div className="text-xs text-gray-400">First seen {formatTime(entry.firstDetectedAt)}</div>}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {waiting && entry.request?.requestId ? (
            <button
              type="button"
              onClick={() => {
                if (entry.request?.requestId) onCancel(entry.request.requestId)
              }}
              disabled={busy}
              className="rounded-full border border-blue-200 bg-white px-3.5 py-1 text-sm text-blue-700 hover:bg-blue-50 disabled:opacity-50"
            >
              Cancel
            </button>
          ) : (
            <button
              type="button"
              onClick={onClear}
              disabled={busy}
              className="rounded-full bg-gray-800 px-3.5 py-1 text-sm font-medium text-white hover:bg-gray-900 disabled:opacity-50"
            >
              Clear now
            </button>
          )}
          <button
            type="button"
            onClick={onForget}
            disabled={busy}
            className="rounded-full border border-red-200 bg-white px-3.5 py-1 text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
          >
            Forget
          </button>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
        <ClearanceBadge entry={entry} />
        {entry.request && <RequestNote request={entry.request} />}
      </div>
    </div>
  )
}

function ClearanceBadge({ entry }: { entry: DomainEntry }) {
  const c = entry.clearance
  if (c.state === 'valid') {
    return <span className="rounded-full bg-green-100 px-2 py-0.5 font-medium text-green-800">Cleared until {formatTime(c.expiresAt)}</span>
  }
  if (c.state === 'expired') {
    return (
      <span className="rounded-full bg-amber-100 px-2 py-0.5 font-medium text-amber-800">Clearance expired {formatTime(c.expiresAt)}</span>
    )
  }
  return <span className="rounded-full bg-gray-100 px-2 py-0.5 font-medium text-gray-700">Not cleared</span>
}

function RequestNote({ request }: { request: NonNullable<DomainEntry['request']> }) {
  switch (request.state) {
    case 'waiting':
      return (
        <span role="status" className="animate-pulse rounded-full bg-blue-100 px-2 py-0.5 font-medium text-blue-800">
          A Chrome window is open: waiting for you to clear it
        </span>
      )
    case 'cleared':
      return <span className="text-gray-500">Cleared {formatTime(request.finishedAt)}.</span>
    case 'gave_up':
      return <span className="text-gray-500">Not cleared: the wait ended {formatTime(request.finishedAt)} with the check still there.</span>
    case 'failed':
      return <span className="text-red-700">The window did not open or was cancelled{request.detail ? `: ${request.detail}` : ''}.</span>
  }
}
