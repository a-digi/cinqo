import { useEffect, useMemo, useState } from 'react'
import { fetchAllAiTraceLogs, type AllAiTraceLogsEntry } from '../../../api/conversations'
import { ApiError } from '../../../api/client'
import { AiLogsStats } from './AiLogsStats'
import { AiLogsTable } from './AiLogsTable'
import { AiLogsToolbar } from './AiLogsToolbar'
import { filterEntries, sortEntries, type SortKey } from './aiLogsRules'

// Admin-wide overview: every conversation that has ever produced an AI
// trace log, where its own folder lives, and how many files exist —
// the "where do I even start looking" entry point, so an admin doesn't
// need to already know a conversation's own ID before finding its
// logs. Never opens/renders a log file's own content. Deliberately no
// link out to the per-conversation AiLogsPage: that page's own
// endpoint is owner-scoped (no cinqo:super:admin bypass, by design —
// see route-conversation.yaml's own "does not grant visibility into
// other users' conversations" note), so a link into a conversation
// this admin doesn't own would 404 — this overview already shows the
// same folder/count inline, so no link adds anything. See
// plan/ai/conversation/step-38-ai-logs-overview-page.md.
const PAGE_SIZE = 25

export function AiLogsOverviewPage() {
  const [entries, setEntries] = useState<AllAiTraceLogsEntry[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [sortKey, setSortKey] = useState<SortKey>('newest')
  const [now] = useState(() => new Date())
  const [query, setQuery] = useState('')
  const [shown, setShown] = useState(PAGE_SIZE)
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
    fetchAllAiTraceLogs()
      .then(setEntries)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  return (
    <div className="cinqo-stagger cinqo-v-swing cinqo-t-type space-y-5 p-6 font-sans text-gray-900">
      <div>
        <h1 className="text-xl font-semibold">AI Logs</h1>
        <p className="mt-1 text-sm text-gray-500">
          Every conversation that has produced an AI trace log, and where its files live. Only filled while AI trace logging is on
          (Settings).
        </p>
        <details className="mt-2 text-sm text-gray-500">
          <summary className="cursor-pointer select-none text-blue-600 hover:underline">How this works</summary>
          <p className="mt-2 max-w-3xl">
            One row per conversation, with the folder holding its log files and how many exist. Nothing here opens a file&apos;s content:
            open the folder directly (a text editor, or the API) to read one.
          </p>
        </details>
      </div>

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
    </div>
  )
}
