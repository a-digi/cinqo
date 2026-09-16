import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { fetchAiTraceLogDetail, fetchAiTraceLogs, type AiTraceLogSummary } from '../../api/conversations'

// Read-only diagnostic viewer — the full raw request/response exchange
// with the AI model, one entry per turn, for understanding excessive
// token usage. Same list + lazy-expand-to-detail-with-cache pattern as
// the Browser tool's own CrawlLogsPage. See
// plan/ai/conversation/step-36-ai-trace-logs.md.
export function AiLogsPage() {
  const { id } = useParams<{ id: string }>()
  const [logs, setLogs] = useState<AiTraceLogSummary[]>([])
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [details, setDetails] = useState<Record<string, string | undefined>>({})
  const [detailErrors, setDetailErrors] = useState<Record<string, string | undefined>>({})
  const [loadingIds, setLoadingIds] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (!id) return
    fetchAiTraceLogs(id)
      .then(setLogs)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [id])

  function toggleExpand(turnRunId: string) {
    const next = expandedId === turnRunId ? null : turnRunId
    setExpandedId(next)
    if (!next || !id || details[next] || loadingIds.has(next)) return

    setLoadingIds((prev) => new Set(prev).add(next))
    setDetailErrors((prev) => ({ ...prev, [next]: undefined }))
    fetchAiTraceLogDetail(id, next)
      .then((detail) => {
        setDetails((prev) => ({ ...prev, [next]: detail }))
      })
      .catch((err: unknown) => {
        setDetailErrors((prev) => ({ ...prev, [next]: err instanceof Error ? err.message : String(err) }))
      })
      .finally(() => {
        setLoadingIds((prev) => {
          const nextSet = new Set(prev)
          nextSet.delete(next)
          return nextSet
        })
      })
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <div className="mb-1.5 flex items-center gap-3">
        <Link to="/conversations" className="text-sm text-gray-500 underline">
          ← Back to conversations
        </Link>
      </div>
      <h1 className="mb-1.5 text-xl font-semibold">AI Trace Logs</h1>
      <p className="mb-5 text-sm text-gray-500">
        The full raw request and response exchanged with the AI model for each turn of this conversation — every iteration of that turn's
        own tool-calling loop, in order. Useful for understanding exactly what's driving token usage.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {logs.length === 0 && !error && <p className="text-sm text-gray-400">No trace logs for this conversation yet.</p>}

      <div className="space-y-2">
        {logs.map((entry) => {
          const expanded = expandedId === entry.turnRunId
          const detail = details[entry.turnRunId]
          return (
            <div key={entry.turnRunId} className="rounded-md border border-gray-200 p-3">
              <button
                type="button"
                onClick={() => {
                  toggleExpand(entry.turnRunId)
                }}
                className="flex w-full items-center justify-between gap-3 text-left"
              >
                <div className="min-w-0">
                  <div className="truncate text-xs font-medium text-gray-900">{entry.turnRunId}</div>
                  <div className="text-xs text-gray-500">
                    {new Date(entry.modifiedAt).toLocaleString()} · {(entry.sizeBytes / 1024).toFixed(1)} KB
                  </div>
                </div>
                <span className="shrink-0 text-xs text-gray-500 underline">{expanded ? 'hide' : 'view'}</span>
              </button>

              {expanded && (
                <div className="mt-3 border-t border-gray-100 pt-3">
                  {loadingIds.has(entry.turnRunId) && <p className="text-xs text-gray-400">Loading…</p>}
                  {detailErrors[entry.turnRunId] && <p className="text-xs text-red-700">{detailErrors[entry.turnRunId]}</p>}
                  {detail && (
                    <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-2 text-xs text-gray-700">
                      {detail}
                    </pre>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
