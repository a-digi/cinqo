import { useEffect, useState } from 'react'
import { fetchConversationSettings, updateConversationSettings, type ConversationSettings } from '../../../api/conversations'
import { ApiError } from '../../../api/client'
import { AiLoggingCard } from './AiLoggingCard'
import { AiLogsContent } from './AiLogsContent'
import { logsView } from './aiLogsRules'

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
//
// Step 44: this page also holds the AI logging switch (it used to be its own Settings page) and shows nothing but
// that card while logging is off — or while the setting is unknown (loading, or it could not be read). The list
// below is mounted, and so requested, only while logging is on.
function messageOf(err: unknown): string {
  return err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err)
}

export function AiLogsOverviewPage() {
  const [settings, setSettings] = useState<ConversationSettings | null>(null)
  const [loadError, setLoadError] = useState('')
  const [saveError, setSaveError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    fetchConversationSettings()
      .then(setSettings)
      .catch((err: unknown) => {
        setLoadError(messageOf(err))
      })
  }, [])

  // The switch only shows the new value once the server has it: a failed save leaves it where it was.
  function change(enabled: boolean) {
    setSaveError('')
    setSaving(true)
    updateConversationSettings({ aiTraceLogsEnabled: enabled })
      .then(setSettings)
      .catch((err: unknown) => {
        setSaveError(messageOf(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  const view = logsView(settings, loadError !== '')

  return (
    <div className="cinqo-stagger cinqo-v-swing cinqo-t-type max-w-5xl space-y-5 p-6 font-sans text-gray-900">
      {view === 'on' && (
        <div>
          <h1 className="text-xl font-semibold">AI Logs</h1>
          <p className="mt-1 text-sm text-gray-500">
            Every conversation that has produced an AI trace log. Open one to read its files here.
          </p>
          <details className="mt-2 text-sm text-gray-500">
            <summary className="cursor-pointer select-none text-blue-600 hover:underline">How this works</summary>
            <p className="mt-2 max-w-3xl">
              One row per conversation, with the folder holding its log files and how many exist. Open shows the newest file in a
              terminal-style reader and lists the others; big files are read a window of up to 500 lines at a time, never loaded whole.
              Opening a file is written to the server log (who and which file, never the content).
            </p>
          </details>
        </div>
      )}
      {view === 'error' ? (
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {loadError}
        </p>
      ) : (
        <AiLoggingCard settings={settings} saving={saving} error={saveError} onChange={change} />
      )}
      {view === 'on' && <AiLogsContent />}
    </div>
  )
}
