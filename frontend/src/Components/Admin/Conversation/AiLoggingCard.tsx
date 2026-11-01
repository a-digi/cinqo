import type { ConversationSettings } from '../../../api/conversations'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { IconBadge } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'

const accent = accents.violet

// The AI logging setting as a Dashboard-style card (the frame of Home/WidgetFrame: rounded, accent border and
// gradient, icon badge, decorative shapes), plan/ai/conversation/step-44-ai-logging-settings-into-logs-page.md.
// `settings` is null while it loads. It turns on whether the conversation feature writes an AI trace log at all
// (step 37): off, nothing is written, for every user's conversations.
export function AiLoggingCard({
  settings,
  saving,
  error,
  onChange,
}: {
  settings: ConversationSettings | null
  saving: boolean
  error: string
  onChange: (enabled: boolean) => void
}) {
  const on = settings?.aiTraceLogsEnabled === true
  return (
    <section
      aria-label="AI logging"
      className="cinqo-card-enter relative overflow-hidden rounded-3xl border p-5 shadow-sm transition-shadow hover:shadow-lg"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape="circles" accent={accent} />
      <div className="relative mb-4 flex items-center gap-3">
        <IconBadge name="document" from={accent.from} to={accent.to} />
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-gray-900">AI logging</h2>
        {settings && (
          <span
            className="rounded-full px-2.5 py-0.5 text-xs font-semibold"
            style={on ? { background: accent.soft, color: accent.text } : { background: '#f3f4f6', color: '#6b7280' }}
          >
            {on ? 'On' : 'Off'}
          </span>
        )}
      </div>
      <div className="relative">
        {settings ? (
          <div className="flex items-start gap-3">
            <button
              type="button"
              role="switch"
              id="cinqo-ai-trace-logs-enabled"
              aria-checked={on}
              aria-labelledby="cinqo-ai-trace-logs-label"
              disabled={saving}
              onClick={() => {
                onChange(!on)
              }}
              className="relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-2 disabled:opacity-50"
              style={{ background: on ? `linear-gradient(135deg, ${accent.from}, ${accent.to})` : '#d1d5db' }}
            >
              <span
                aria-hidden="true"
                className={`inline-block h-5 w-5 rounded-full bg-white shadow transition-transform ${on ? 'translate-x-5' : 'translate-x-0.5'}`}
              />
            </button>
            <div className="min-w-0">
              <p id="cinqo-ai-trace-logs-label" className="text-sm font-medium text-gray-900">
                Log every AI turn
              </p>
              <p className="mt-1 text-xs text-gray-500">
                Writes the full request and answer exchanged with the AI model to disk, one file per turn, for every conversation. Off by
                default. This affects every user&apos;s conversations at once, not just yours. While off, nothing is written and this page
                shows no logs.
              </p>
            </div>
          </div>
        ) : (
          <div className="flex animate-pulse flex-col gap-3" aria-label="Loading">
            <div className="h-6 w-44 rounded-full" style={{ background: accent.soft }} />
            <div className="h-3 w-2/3 rounded-full" style={{ background: accent.soft }} />
          </div>
        )}
        {error && (
          <p role="alert" className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
            {error}
          </p>
        )}
      </div>
    </section>
  )
}
