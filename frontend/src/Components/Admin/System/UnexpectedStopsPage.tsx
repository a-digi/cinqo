import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { ApiError } from '../../../api/client'
import {
  acknowledgeAllSystemEvents,
  fetchSystemEventsPage,
  fetchSystemEventsSummary,
  type SystemEventsPage,
  type SystemEventsSummary,
} from '../../../api/system'
import { canManageSystem } from '../../../Layout/agentUnload'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { IconBadge } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { Pagination } from '../../../Shared/Components/Pagination/Pagination'
import { clampPage } from '../../../Shared/Components/Pagination/paginationRules'
import { useAuth } from '../../Auth/AuthContext'
import { StopCard, StopListHeader } from './StopCard'
import { StopsEmpty, StopsError, StopSkeletons } from './StopStates'
import { StopStats } from './StopStats'
import { STOP_PAGE_SIZES, listQueryString, readListQuery } from './unexpectedStopsRules'

// The dashboard of the unexpected stops (plan 259): every time Cinqo or the local model stopped without being asked to, newest first, as cards in a list with
// pagination, under a title card with three tiles. The place in the list is in the address (?page=&pageSize=), so a reload, a link and the Back button keep it;
// a card opens the page of that one stop. A person with the manage scope can mark every new stop as seen.
export function UnexpectedStopsPage() {
  const [params, setParams] = useSearchParams()
  const { page, pageSize } = readListQuery(params)
  const { scopes } = useAuth()
  const canManage = canManageSystem(scopes)
  const { confirm, dialog } = useConfirm()
  const [data, setData] = useState<SystemEventsPage | null>(null)
  const [summary, setSummary] = useState<SystemEventsSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloads, setReloads] = useState(0)
  const [marking, setMarking] = useState(false)
  const [notice, setNotice] = useState('')
  const rose = accents.rose

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    fetchSystemEventsPage(page, pageSize, controller.signal)
      .then((read) => {
        if (controller.signal.aborted) return
        // The address names a page past the last one (the stops were trimmed, or the address was typed): the last page is shown instead.
        const last = clampPage(page, read.total, pageSize)
        if (read.events.length === 0 && read.total > 0 && last !== page) {
          setParams(listQueryString(last, pageSize), { replace: true })
          return
        }
        setData(read)
        setLoading(false)
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return
        setError(e instanceof ApiError || e instanceof Error ? e.message : 'The unexpected stops could not be read.')
        setLoading(false)
      })
    // the number of new stops is only for the tiles: it never stops the list
    fetchSystemEventsSummary(controller.signal)
      .then(setSummary)
      .catch(() => undefined)
    return () => {
      controller.abort()
    }
  }, [page, pageSize, reloads, setParams])

  const go = useCallback(
    (nextPage: number, nextSize: number) => {
      setParams(listQueryString(nextPage, nextSize))
    },
    [setParams],
  )

  const reload = () => {
    setReloads((n) => n + 1)
  }

  const markAll = async () => {
    const unseen = summary?.unseen ?? 0
    const ok = await confirm({
      title: 'Mark all as seen?',
      message: `${String(unseen)} new ${unseen === 1 ? 'stop' : 'stops'} will be marked as seen. This changes the record.`,
      confirmLabel: 'Mark all as seen',
    })
    if (!ok) return
    setMarking(true)
    setNotice('')
    try {
      const marked = await acknowledgeAllSystemEvents()
      setNotice(`${String(marked)} ${marked === 1 ? 'stop' : 'stops'} marked as seen.`)
      reload()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'The stops could not be marked as seen.')
    } finally {
      setMarking(false)
    }
  }

  const unseen = summary?.unseen ?? 0
  return (
    <div className="cinqo-v-drop cinqo-stagger max-w-6xl space-y-6 p-6">
      <section
        aria-label="Overview"
        className="relative overflow-hidden rounded-3xl border p-6 shadow-sm"
        style={{ borderColor: rose.soft, background: `linear-gradient(160deg, #ffffff 35%, ${rose.soft} 100%)` }}
      >
        <Decor shape="ring" accent={rose} />
        <div className="relative flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            <IconBadge name="alert" from={rose.from} to={rose.to} />
            <div>
              <h1 className="text-xl font-bold text-gray-900">Unexpected stops</h1>
              <p className="mt-0.5 text-sm text-gray-600">
                Every time Cinqo or the local model stopped without being asked to, newest first.
              </p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {canManage && unseen > 0 && (
              <button
                type="button"
                disabled={marking}
                onClick={() => void markAll()}
                className="rounded-full px-4 py-1.5 text-xs font-semibold text-white shadow-sm transition-transform hover:-translate-y-0.5 disabled:opacity-60"
                style={{ background: `linear-gradient(135deg, ${rose.from}, ${rose.to})` }}
              >
                {marking ? 'Marking…' : 'Mark all as seen'}
              </button>
            )}
            <button
              type="button"
              disabled={loading}
              onClick={reload}
              className="inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-white px-4 py-1.5 text-xs font-semibold text-gray-700 shadow-sm transition-transform hover:-translate-y-0.5 hover:bg-gray-50 disabled:opacity-60"
            >
              <svg
                aria-hidden="true"
                viewBox="0 0 20 20"
                className={`h-3.5 w-3.5 ${loading ? 'animate-spin motion-reduce:animate-none' : ''}`}
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M16 10a6 6 0 1 1-1.8-4.3M16 3.5V6h-2.5" />
              </svg>
              {loading && data !== null ? 'Refreshing…' : 'Refresh'}
            </button>
          </div>
        </div>
        <StopStats total={data?.total ?? 0} summary={summary} />
        {notice !== '' && (
          <p
            role="status"
            className="cinqo-stop-in relative mt-4 inline-flex items-center gap-2 rounded-full bg-green-100 px-3 py-1 text-xs font-semibold text-green-800"
          >
            ✓ {notice}
          </p>
        )}
      </section>

      {error !== null && <StopsError message={error} onRetry={reload} />}

      {data === null && error === null && <StopSkeletons />}

      {data !== null && data.total === 0 && <StopsEmpty />}

      {data !== null && data.total > 0 && (
        <section aria-label="Stops" className="space-y-3">
          <div
            role="table"
            aria-label="Unexpected stops, newest first"
            aria-busy={loading}
            className={`space-y-3 transition-opacity ${loading ? 'opacity-60' : ''}`}
          >
            <StopListHeader />
            <div key={`${String(page)}-${String(pageSize)}`} role="rowgroup" className="space-y-3">
              {data.events.map((e, i) => (
                <StopCard key={e.id} stop={e} index={i} />
              ))}
            </div>
          </div>
          <div className="rounded-2xl border border-gray-100 bg-white p-3 shadow-sm">
            <Pagination
              page={page}
              pageSize={pageSize}
              total={data.total}
              pageSizes={STOP_PAGE_SIZES}
              label="Unexpected stops pages"
              accent={{ from: rose.from, to: rose.to }}
              onPageChange={(p) => {
                go(p, pageSize)
              }}
              onPageSizeChange={(s) => {
                go(1, s)
              }}
            />
          </div>
        </section>
      )}
      {dialog}
    </div>
  )
}
