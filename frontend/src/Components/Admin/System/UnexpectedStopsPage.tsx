import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ApiError } from '../../../api/client'
import { fetchSystemEventsPage, fetchSystemEventsSummary, type SystemEventsPage, type SystemEventsSummary } from '../../../api/system'
import { unexpectedStopPath } from '../../../config/routing/systemPaths'
import { eventTitle } from '../../../Layout/eventsRules'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { Pagination } from '../../../Shared/Components/Pagination/Pagination'
import { clampPage } from '../../../Shared/Components/Pagination/paginationRules'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { STOP_PAGE_SIZES, countsLine, formatDateTime, listQueryString, readListQuery, seenTitle, statusLabel } from './unexpectedStopsRules'

// The dashboard of the unexpected stops (plan 259): every time Cinqo or the local model stopped without being asked to, newest first, in a table with
// pagination. The place in the table is in the address (?page=&pageSize=), so a reload, a link and the Back button keep it; a row opens the page of that one stop.
export function UnexpectedStopsPage() {
  const [params, setParams] = useSearchParams()
  const { page, pageSize } = readListQuery(params)
  const [data, setData] = useState<SystemEventsPage | null>(null)
  const [summary, setSummary] = useState<SystemEventsSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloads, setReloads] = useState(0)

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
    // the number of new stops is only for the line under the title: it never stops the table
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

  return (
    <div className="max-w-6xl space-y-5 p-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Unexpected stops</h1>
          <p className="mt-1 text-sm text-gray-600">Every time Cinqo or the local model stopped without being asked to, newest first.</p>
          {data !== null && (
            <p data-testid="stops-counts" className="mt-1 text-xs font-medium text-gray-500">
              {countsLine(data.total, summary?.unseen ?? null)}
            </p>
          )}
        </div>
        <button
          type="button"
          disabled={loading}
          onClick={() => {
            setReloads((n) => n + 1)
          }}
          className="rounded-md border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
        >
          {loading && data !== null ? 'Refreshing…' : 'Refresh'}
        </button>
      </header>

      {error !== null && (
        <p role="alert" className="flex flex-wrap items-center gap-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
          <button
            type="button"
            onClick={() => {
              setReloads((n) => n + 1)
            }}
            className="rounded-md border border-red-200 bg-white px-2.5 py-1 text-xs font-medium text-red-700 hover:bg-red-100"
          >
            Retry
          </button>
        </p>
      )}

      {data === null && error === null && (
        <div className="flex items-center gap-2 text-sm text-gray-500">
          <LoadingSpinner /> Loading…
        </div>
      )}

      {data !== null && data.total === 0 && (
        <p className="rounded-lg border border-gray-100 bg-gray-50 p-8 text-center text-sm text-gray-500">No unexpected stops recorded.</p>
      )}

      {data !== null && data.total > 0 && (
        <>
          <div className={`overflow-x-auto rounded-lg border border-gray-200 bg-white ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
            <table className="w-full text-left text-sm">
              <caption className="sr-only">Unexpected stops, newest first</caption>
              <thead>
                <tr className="border-b border-gray-200 bg-gray-50 text-xs text-gray-500">
                  <th scope="col" className="px-4 py-2 font-medium">
                    When
                  </th>
                  <th scope="col" className="px-4 py-2 font-medium">
                    What
                  </th>
                  <th scope="col" className="px-4 py-2 font-medium">
                    Why (the app&apos;s guess)
                  </th>
                  <th scope="col" className="px-4 py-2 font-medium">
                    Status
                  </th>
                  <th scope="col" className="px-4 py-2">
                    <span className="sr-only">Details</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.events.map((e) => (
                  <tr key={e.id} data-testid="stop-row" className="border-b border-gray-100 last:border-0 hover:bg-gray-50">
                    <td className="whitespace-nowrap px-4 py-2">
                      <Link to={unexpectedStopPath(e.id)} className="font-medium text-blue-700 hover:underline">
                        {formatDateTime(e.at)}
                      </Link>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-gray-800">{eventTitle(e)}</td>
                    <td className="max-w-md px-4 py-2 text-gray-600">
                      <span className="block truncate" title={e.verdict}>
                        {e.verdict}
                      </span>
                    </td>
                    <td className="px-4 py-2">
                      <Pill variant={e.acknowledged ? 'gray' : 'red'} title={seenTitle(e) || undefined}>
                        {statusLabel(e)}
                      </Pill>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 text-right">
                      <Link
                        to={unexpectedStopPath(e.id)}
                        className="text-xs font-medium text-gray-700 hover:underline"
                        aria-label={`Details of the stop at ${formatDateTime(e.at)}`}
                      >
                        Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination
            page={page}
            pageSize={pageSize}
            total={data.total}
            pageSizes={STOP_PAGE_SIZES}
            label="Unexpected stops pages"
            onPageChange={(p) => {
              go(p, pageSize)
            }}
            onPageSizeChange={(s) => {
              go(1, s)
            }}
          />
        </>
      )}
    </div>
  )
}
