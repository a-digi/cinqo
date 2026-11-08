import { useEffect, useState } from 'react'
import { fetchAllAiTraceLogs, type AllAiTraceLogsEntry } from '../../../api/conversations'
import { ApiError } from '../../../api/client'

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
export function AiLogsOverviewPage() {
  const [entries, setEntries] = useState<AllAiTraceLogsEntry[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    fetchAllAiTraceLogs()
      .then(setEntries)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err))
      })
  }, [])

  return (
    <div className="max-w-4xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">AI Logs</h1>
      <p className="mb-5 text-sm text-gray-500">
        Every conversation that has ever produced an AI trace log — one row per conversation, showing where its own log files live and how
        many exist. Only populated while AI trace logging is turned on (Settings). Nothing here opens a file's own content; open the folder
        directly (a text editor, or the API) to read one.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {entries.length === 0 && !error && <p className="text-sm text-gray-400">No conversation has produced an AI trace log yet.</p>}

      {entries.length > 0 && (
        <div className="overflow-hidden rounded-md border border-gray-200">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50 text-xs uppercase text-gray-500">
                <th className="px-4 py-2 font-medium">Conversation</th>
                <th className="px-4 py-2 font-medium">Folder</th>
                <th className="px-4 py-2 font-medium">Files</th>
                <th className="px-4 py-2 font-medium">Last modified</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr key={e.conversationId} className="border-b border-gray-100 last:border-0">
                  <td className="px-4 py-2 text-xs text-gray-900">
                    <div className="truncate">{e.title ?? '(deleted conversation)'}</div>
                    <div className="font-mono text-gray-400">{e.conversationId}</div>
                  </td>
                  <td className="max-w-xs truncate px-4 py-2 font-mono text-xs text-gray-600" title={e.folder}>
                    {e.folder}
                  </td>
                  <td className="px-4 py-2 text-xs text-gray-600">{e.logCount}</td>
                  <td className="px-4 py-2 text-xs text-gray-500">{e.modifiedAt ? new Date(e.modifiedAt).toLocaleString() : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
