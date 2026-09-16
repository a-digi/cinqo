import { useEffect, useState } from 'react'
import { fetchConversationSettings, updateConversationSettings, type ConversationSettings } from '../../../api/conversations'
import { ApiError } from '../../../api/client'

// Admin-only toggle controlling whether the conversation feature ever
// writes an AI trace log at all (plan/ai/conversation/step-37-ai-debug-
// logging-toggle.md) — off by default, mirroring the Browser tool's
// own Debug page (tools/browser/frontend-app/src/DebugPage.tsx): when
// off, a turn's own tool-calling loop produces no trace file
// whatsoever, not merely a hidden one.
export function AiDebugSettingsPage() {
  const [settings, setSettings] = useState<ConversationSettings | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    fetchConversationSettings()
      .then(setSettings)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err))
      })
  }, [])

  function update(next: ConversationSettings) {
    setError('')
    setSaving(true)
    setSettings(next) // optimistic — reverted below if the save fails
    updateConversationSettings(next)
      .then(setSettings)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  if (!settings) {
    return (
      <div className="max-w-2xl p-6 font-sans text-gray-900">
        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
      </div>
    )
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">AI Debug Logging</h1>
      <p className="mb-5 text-sm text-gray-500">
        Controls whether the full raw request and response exchanged with the AI model is ever written to disk, one file per turn, for every
        conversation. Off by default — this affects every user's conversations at once, not just your own.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="rounded-md border border-gray-200 bg-gray-50 p-6 shadow-sm">
        <label htmlFor="cinqo-ai-trace-logs-enabled" aria-label="AI trace logging" className="flex items-start gap-3">
          <input
            id="cinqo-ai-trace-logs-enabled"
            type="checkbox"
            checked={settings.aiTraceLogsEnabled}
            disabled={saving}
            onChange={(e) => {
              update({ aiTraceLogsEnabled: e.target.checked })
            }}
            className="mt-0.5 h-4 w-4 rounded border-gray-300"
          />
          <span>
            <span className="block text-sm font-medium">AI trace logging</span>
            <span className="block text-xs text-gray-500">
              When off, no turn is ever logged — nothing is written to disk, not just hidden from view. When on, each conversation's own
              trace files can be found via that conversation's "View AI logs" action.
            </span>
          </span>
        </label>
      </div>
    </div>
  )
}
