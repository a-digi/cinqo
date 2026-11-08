import { useEffect, useMemo, useState } from 'react'
import { fetchAllAiTraceLogs, type AllAiTraceLogsEntry } from '../../../api/conversations'
import { ApiError } from '../../../api/client'
import { AiLogsStats } from './AiLogsStats'
import { AiLogsTable } from './AiLogsTable'
import { AiLogsToolbar } from './AiLogsToolbar'
import { AiLogViewerModal } from './AiLogViewerModal'
import { filterEntries, sortEntries, type SortKey } from './aiLogsRules'

// The list part of the AI Logs page: mounted only while AI trace logging is on (see AiLogsOverviewPage), so it only
// asks for the entries then, and starts afresh every time logging is turned on.
const PAGE_SIZE = 25

export function AiLogsContent() {
  const [entries, setEntries] = useState<AllAiTraceLogsEntry[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [sortKey, setSortKey] = useState<SortKey>('newest')
  const [now] = useState(() => new Date())
  const [query, setQuery] = useState('')
  const [shown, setShown] = useState(PAGE_SIZE)
  const [opened, setOpened] = useState<AllAiTraceLogsEntry | null>(null)
  const sorted = useMemo(() => sortEntries(filterEntries(entries, query), sortKey), [entries, query, sortKey])
  const visible = sorted.slice(0, shown)

  // A new search or sort starts again from the first page.
  const changeQuery = (value: string) => {
    setQuery(value)
    setShown(PAGE_SIZE)
  }
  const changeSort = (key: SortKey) => {
    setSortKey(key)
    setShown(PAGE_SIZE)
  }

  useEffect(() => {
    // Logging turned off meanwhile: this component is unmounted and the late answer is dropped.
    let cancelled = false
    fetchAllAiTraceLogs()
      .then((list) => {
        if (!cancelled) setEntries(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="space-y-5">
      {error ? (
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {error}
        </p>
      ) : (
        <>
          <AiLogsStats entries={entries} loading={loading} now={now} />
          <AiLogsToolbar
            query={query}
            onQueryChange={changeQuery}
            sortKey={sortKey}
            onSortChange={changeSort}
            matching={sorted.length}
            total={entries.length}
          />
          <AiLogsTable
            entries={visible}
            loading={loading}
            query={query}
            onClearQuery={() => {
              changeQuery('')
            }}
            sortKey={sortKey}
            onSortChange={changeSort}
            now={now}
            onOpen={setOpened}
          />
          {sorted.length > shown && (
            <div className="flex items-center justify-center gap-3 text-sm text-gray-500">
              <span>
                Showing {shown} of {sorted.length}
              </span>
              <button
                type="button"
                onClick={() => {
                  setShown((n) => n + PAGE_SIZE)
                }}
                className="rounded-lg border border-gray-300 bg-white px-4 py-2 font-medium text-gray-800 hover:bg-gray-50"
              >
                Show {Math.min(PAGE_SIZE, sorted.length - shown)} more
              </button>
            </div>
          )}
        </>
      )}
      {opened && (
        <AiLogViewerModal
          entry={opened}
          onClose={() => {
            setOpened(null)
          }}
        />
      )}
    </div>
  )
}
