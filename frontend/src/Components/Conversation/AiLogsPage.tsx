import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { fetchAiTraceLogs, type AiTraceLogSummary } from '../../api/conversations'

// Read-only summary — deliberately shows only WHERE this conversation's
// own AI trace logs live on disk and HOW MANY exist, never opens/
// renders a file's own content (that's a job for a text editor/curl
// against the folder path shown here, not this page). One row per
// turn: its own id, size, and last-modified time. See
// plan/ai/conversation/step-36-ai-trace-logs.md and
// plan/ai/conversation/step-37-ai-debug-logging-toggle.md (the setting
// that gates whether these files get written at all).
export function AiLogsPage() {
  const { id } = useParams<{ id: string }>()
  const [folder, setFolder] = useState('')
  const [logs, setLogs] = useState<AiTraceLogSummary[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) return
    fetchAiTraceLogs(id)
      .then((result) => {
        setFolder(result.folder)
        setLogs(result.logs)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [id])

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <div className="mb-1.5 flex items-center gap-3">
        <Link to="/conversations" className="text-sm text-gray-500 underline">
          ← Back to conversations
        </Link>
      </div>
      <h1 className="mb-1.5 text-xl font-semibold">AI Trace Logs</h1>
      <p className="mb-5 text-sm text-gray-500">
        Where this conversation's own AI request/response trace files live — one file per turn, only written while AI debug logging is
        turned on (see Admin → AI Debug Logging). Open a file directly (a text editor, or the API) to read it; this page only reports what
        exists and where.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {folder && (
        <div className="mb-4 rounded-md border border-gray-200 bg-gray-50 p-3">
          <div className="text-xs font-medium text-gray-500">Folder</div>
          <div className="break-all font-mono text-sm text-gray-900">{folder}</div>
          <div className="mt-1 text-xs text-gray-500">
            {logs.length} log file{logs.length === 1 ? '' : 's'}
          </div>
        </div>
      )}

      {folder && logs.length === 0 && !error && (
        <p className="text-sm text-gray-400">No trace logs for this conversation yet — either none exist, or AI debug logging was off.</p>
      )}

      {logs.length > 0 && (
        <div className="overflow-hidden rounded-md border border-gray-200">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50 text-xs uppercase text-gray-500">
                <th className="px-4 py-2 font-medium">Turn (prompt)</th>
                <th className="px-4 py-2 font-medium">Size</th>
                <th className="px-4 py-2 font-medium">Last modified</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((entry) => (
                <tr key={entry.turnRunId} className="border-b border-gray-100 last:border-0">
                  <td className="px-4 py-2 font-mono text-xs">{entry.turnRunId}.txt</td>
                  <td className="px-4 py-2 text-xs text-gray-600">{(entry.sizeBytes / 1024).toFixed(1)} KB</td>
                  <td className="px-4 py-2 text-xs text-gray-500">{new Date(entry.modifiedAt).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
