import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ApiError } from '../../../api/client'
import { acknowledgeSystemEvent, fetchSystemEvent, type SystemEvent } from '../../../api/system'
import { UNEXPECTED_STOPS_PATH } from '../../../config/routing/systemPaths'
import { canManageSystem } from '../../../Layout/agentUnload'
import { eventTitle } from '../../../Layout/eventsRules'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { WidgetIconGlyph } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'
import { useAuth } from '../../Auth/AuthContext'
import { StopsError } from './StopStates'
import { formatDateTime, kindLook, relativeTime, seenTitle } from './unexpectedStopsRules'

function BackLink() {
  return (
    <Link
      to={UNEXPECTED_STOPS_PATH}
      className="group inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-white px-3.5 py-1.5 text-xs font-semibold text-gray-700 shadow-sm transition-transform hover:-translate-y-0.5 hover:bg-gray-50"
    >
      <span aria-hidden="true" className="transition-transform duration-200 group-hover:-translate-x-1">
        ←
      </span>
      All unexpected stops
    </Link>
  )
}

function Skeleton() {
  return (
    <div role="status" aria-label="Loading" className="animate-pulse space-y-5">
      <div className="h-36 rounded-3xl border border-gray-100 bg-white" />
      <div className="h-28 rounded-3xl border border-gray-100 bg-white" />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-16 rounded-2xl border border-gray-100 bg-white" />
        ))}
      </div>
    </div>
  )
}

// A stop that is not there: it never was (a made-up address) or it was trimmed away (only the newest stops are kept).
function Missing() {
  const a = accents.amber
  return (
    <div
      data-testid="stop-missing"
      className="cinqo-stop-in relative overflow-hidden rounded-3xl border p-10 text-center shadow-sm"
      style={{ borderColor: a.soft, background: `linear-gradient(160deg, #ffffff 40%, ${a.soft} 100%)` }}
    >
      <Decor shape="diamond" accent={a} />
      <div className="relative mx-auto flex max-w-md flex-col items-center gap-3">
        <span
          className="cinqo-bob inline-flex h-16 w-16 items-center justify-center rounded-3xl text-white shadow-md"
          style={{ background: `linear-gradient(135deg, ${a.from}, ${a.to})` }}
        >
          <WidgetIconGlyph name="alert" className="h-8 w-8" />
        </span>
        <h2 className="text-lg font-semibold text-gray-900">This stop does not exist any more</h2>
        <p className="text-sm text-gray-600">It may have been removed: Cinqo keeps only the newest unexpected stops.</p>
        <BackLink />
      </div>
    </div>
  )
}

// The page of one unexpected stop (plan 259), by its UUID: when it happened, which kind of stop it was, the app's own guess at why, and every fact it recorded.
// A person with the manage scope can mark a new stop as seen here.
export function UnexpectedStopPage() {
  const { id = '' } = useParams()
  const { scopes } = useAuth()
  const canManage = canManageSystem(scopes)
  const [stop, setStop] = useState<SystemEvent | null>(null)
  const [missing, setMissing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [reloads, setReloads] = useState(0)
  const [marking, setMarking] = useState(false)
  const [marked, setMarked] = useState(false)
  const [markError, setMarkError] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    setMissing(false)
    fetchSystemEvent(id, controller.signal)
      .then((read) => {
        if (controller.signal.aborted) return
        setStop(read)
        setLoading(false)
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return
        if (e instanceof ApiError && (e.status === 404 || e.status === 400)) setMissing(true)
        else setError(e instanceof Error ? e.message : 'The unexpected stop could not be read.')
        setStop(null)
        setLoading(false)
      })
    return () => {
      controller.abort()
    }
  }, [id, reloads])

  const reload = useCallback(() => {
    setReloads((n) => n + 1)
  }, [])

  const markSeen = async () => {
    setMarking(true)
    setMarkError('')
    try {
      await acknowledgeSystemEvent(id)
      setMarked(true)
      reload()
    } catch (e) {
      setMarkError(e instanceof Error ? e.message : 'The stop could not be marked as seen.')
    } finally {
      setMarking(false)
    }
  }

  const look = kindLook(stop?.kind ?? '')
  const accent = accents[look.accent]
  const unseen = stop !== null && !stop.acknowledged

  return (
    <div className="cinqo-v-drop cinqo-stagger max-w-4xl space-y-6 p-6">
      <div>
        <BackLink />
      </div>

      {loading && stop === null && !missing && error === null && <Skeleton />}
      {missing && <Missing />}
      {error !== null && <StopsError message={error} onRetry={reload} />}

      {stop !== null && (
        <>
          <section
            aria-label="Stop"
            className="relative overflow-hidden rounded-3xl border p-6 shadow-sm"
            style={{
              borderColor: unseen ? accent.from : accent.soft,
              background: `linear-gradient(160deg, #ffffff 35%, ${accent.soft} 100%)`,
            }}
          >
            <Decor shape="ring" accent={accent} />
            {unseen && (
              <span
                aria-hidden="true"
                data-testid="stop-bar"
                className="absolute inset-y-0 left-0 w-1.5"
                style={{ background: `linear-gradient(180deg, ${accent.from}, ${accent.to})` }}
              />
            )}
            <div className="relative flex flex-wrap items-center justify-between gap-5">
              <div className="flex items-center gap-4">
                <span
                  className="cinqo-v-pop inline-flex h-14 w-14 shrink-0 items-center justify-center rounded-3xl text-white shadow-md"
                  style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
                >
                  <WidgetIconGlyph name={look.icon} className="h-7 w-7" />
                </span>
                <div>
                  <h1 className="text-xl font-bold text-gray-900">{eventTitle(stop)}</h1>
                  <p className="mt-0.5 text-sm font-medium text-gray-700">{formatDateTime(stop.at)}</p>
                  <p className="text-xs text-gray-500">{relativeTime(stop.at)}</p>
                </div>
              </div>
              <div className="flex flex-col items-end gap-2">
                {unseen ? (
                  <span
                    data-testid="stop-status"
                    className="inline-flex items-center gap-1.5 rounded-full bg-red-100 px-3 py-1 text-xs font-semibold text-red-700"
                  >
                    <span aria-hidden="true" className="relative flex h-2 w-2">
                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75 motion-reduce:animate-none" />
                      <span className="relative inline-flex h-2 w-2 rounded-full bg-red-500" />
                    </span>
                    New
                  </span>
                ) : (
                  <span
                    data-testid="stop-status"
                    className="cinqo-v-pop inline-flex items-center gap-1.5 rounded-full bg-green-100 px-3 py-1 text-xs font-semibold text-green-800"
                  >
                    ✓ {seenTitle(stop) || 'Seen'}
                  </span>
                )}
                {unseen && canManage && (
                  <button
                    type="button"
                    disabled={marking}
                    onClick={() => void markSeen()}
                    className="rounded-full px-4 py-1.5 text-xs font-semibold text-white shadow-sm transition-transform hover:-translate-y-0.5 disabled:opacity-60"
                    style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
                  >
                    {marking ? 'Marking…' : 'Mark as seen'}
                  </button>
                )}
                {markError !== '' && (
                  <p role="alert" className="max-w-xs text-right text-xs text-red-700">
                    {markError}
                  </p>
                )}
              </div>
            </div>
            {marked && !unseen && (
              <p
                role="status"
                className="cinqo-stop-in relative mt-4 inline-flex items-center gap-2 rounded-full bg-green-100 px-3 py-1 text-xs font-semibold text-green-800"
              >
                ✓ Marked as seen.
              </p>
            )}
          </section>

          <section
            aria-label="What the app thinks happened"
            className="relative overflow-hidden rounded-3xl border border-gray-100 bg-white p-6 shadow-sm"
          >
            <span
              aria-hidden="true"
              className="pointer-events-none absolute -left-1 -top-6 select-none font-serif text-8xl leading-none"
              style={{ color: accent.from, opacity: 0.18 }}
            >
              “
            </span>
            <h2 className="relative text-[11px] font-semibold uppercase tracking-wider text-gray-400">What the app thinks happened</h2>
            <p data-testid="stop-verdict" className="relative mt-2 text-base leading-relaxed text-gray-800">
              {stop.verdict}
            </p>
          </section>

          <section aria-label="Facts" className="space-y-3">
            <h2 className="text-[11px] font-semibold uppercase tracking-wider text-gray-400">Facts the app recorded</h2>
            {stop.evidence.length === 0 ? (
              <p className="rounded-2xl border border-gray-100 bg-gray-50 p-4 text-sm text-gray-500">
                The app recorded no facts for this stop.
              </p>
            ) : (
              <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {stop.evidence.map((f, i) => (
                  <div
                    key={`${f.label}-${String(i)}`}
                    data-testid="stop-fact"
                    className="cinqo-stop-in rounded-2xl border bg-white p-4 shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md"
                    style={{ '--i': i, borderColor: accent.soft, borderLeft: `4px solid ${accent.from}` } as React.CSSProperties}
                  >
                    <dt className="text-[11px] font-semibold uppercase tracking-wider text-gray-400">{f.label}</dt>
                    <dd className="mt-1 break-words text-sm font-semibold text-gray-900">{f.value}</dd>
                  </div>
                ))}
              </dl>
            )}
          </section>

          <p className="text-xs text-gray-400">
            Stop ID <span className="font-mono">{stop.id}</span>
          </p>
        </>
      )}
    </div>
  )
}
