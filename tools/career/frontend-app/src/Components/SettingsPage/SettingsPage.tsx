import { useEffect, useState } from 'react'
import { saveAiSettings } from '../../api'
import { defaultModelFor } from '../../Cinqo/Platform/aiChoice'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { Dropdown } from '../Dropdown/Dropdown'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { RobotIcon } from '../../Shared/Icons/icons'
import { ACCENTS } from '../JobsPage/jobsTheme'

const ACCENT = ACCENTS[4] // violet, like the AI cards on Home

// What the choice is used for, so nobody has to wonder what changing it affects.
const USED_FOR = [
  'Crawling job portals with AI, and preparing their crawl instructions',
  'Job Match: comparing a job with your profile',
  'Generate CV for a job',
  'Import from CV: reading a CV into your profile',
]

// Settings — the one AI platform and model the whole Career tool uses. It used to be
// picked separately (and forgotten on every page load) on the Jobs, Portals and
// Import CV pages; now it is chosen once here, saved, and used everywhere. Nothing
// chosen means "the first platform that has an API key". The keys themselves are
// managed in Cinqo, not here.
export function SettingsPage() {
  const ai = useAiChoice()
  const [platformId, setPlatformId] = useState('')
  const [model, setModel] = useState('')
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  // Start the form from what is in effect (the saved choice, or the automatic pick).
  useEffect(() => {
    if (!ai.loaded) return
    setPlatformId(ai.platform?.id ?? '')
    setModel(ai.model ?? '')
  }, [ai.loaded, ai.platform, ai.model])

  const selected = ai.platforms.find((p) => p.id === platformId) ?? null
  const unchanged = ai.source === 'saved' && platformId === (ai.platform?.id ?? '') && model === (ai.model ?? '')

  function pickPlatform(id: string) {
    setPlatformId(id)
    const next = ai.platforms.find((p) => p.id === id)
    setModel(next ? defaultModelFor(next) : '')
    setMessage(null)
  }

  function save(next: { platformId: string; model: string }, doneText: string) {
    setSaving(true)
    setMessage(null)
    saveAiSettings(next)
      .then(() => {
        setMessage({ kind: 'ok', text: doneText })
        ai.reload()
      })
      .catch((err: unknown) => {
        setMessage({ kind: 'error', text: err instanceof Error ? err.message : String(err) })
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <div className="career-v-spin career-a-sheen career-t-type max-w-3xl p-6 font-sans text-gray-900">
      <h1 className="text-xl font-semibold">Settings</h1>
      <p className="mt-1 mb-5 text-sm text-gray-500">How the Career tool uses AI.</p>

      <section
        className="career-enter career-delay-1 relative overflow-hidden rounded-3xl border p-6 shadow-sm"
        style={{ borderColor: ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${ACCENT.soft} 100%)` }}
      >
        <div className="pointer-events-none absolute -right-8 -top-8 h-32 w-32 rounded-full opacity-15" style={{ background: ACCENT.to }} />
        <div
          className="pointer-events-none absolute -bottom-6 right-24 h-14 w-14 rotate-45 rounded-2xl opacity-10"
          style={{ background: ACCENT.to }}
        />

        <div className="relative flex items-center gap-3">
          <span
            className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm"
            style={{ background: ACCENT.to }}
          >
            <RobotIcon className="h-6 w-6" />
          </span>
          <div>
            <h2 className="text-base font-semibold">AI platform and model</h2>
            <p className="text-sm text-gray-500">One choice for everything below, saved for this installation.</p>
          </div>
        </div>

        {!ai.loaded && <p className="relative mt-5 text-sm text-gray-500">Loading…</p>}

        {ai.loaded && ai.platforms.length === 0 && (
          <div className="relative mt-5">
            <InfoBox
              variant="warning"
              label="No AI platform"
              message="No AI platform is configured in Cinqo yet. Ask an administrator to add one (with an API key) under the AI menu."
            />
          </div>
        )}

        {ai.loaded && ai.platforms.length > 0 && (
          <div className="relative mt-5 space-y-4">
            {ai.source === 'stale' && (
              <InfoBox
                variant="warning"
                label="Saved platform not found"
                message="The platform saved earlier is no longer available, so the first platform with a key is used until you save a new choice."
              />
            )}
            <div className="flex flex-wrap items-end gap-4 [&>div]:min-w-0">
              <div className="w-60 [&>div]:block [&>div>button]:w-full">
                <Dropdown
                  label="Platform"
                  options={ai.platforms.map((p) => ({ value: p.id, label: p.name }))}
                  value={platformId || null}
                  onChange={pickPlatform}
                />
              </div>
              {selected && selected.models.length > 0 && (
                <div className="w-72 [&>div]:block [&>div>button]:w-full">
                  <Dropdown
                    label="Model"
                    options={selected.models.map((m) => ({ value: m, label: m }))}
                    value={model || null}
                    onChange={(m) => {
                      setModel(m)
                      setMessage(null)
                    }}
                  />
                </div>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-3">
              <button
                type="button"
                disabled={saving || platformId === '' || unchanged}
                onClick={() => {
                  save({ platformId, model }, 'Saved.')
                }}
                className="rounded-full px-5 py-2 text-sm font-medium text-white shadow-sm disabled:opacity-50"
                style={{ background: ACCENT.to }}
              >
                {saving ? 'Saving…' : 'Save'}
              </button>
              {ai.source === 'saved' && (
                <button
                  type="button"
                  disabled={saving}
                  onClick={() => {
                    save({ platformId: '', model: '' }, 'Back to automatic: the first platform with a key is used.')
                  }}
                  className="text-sm text-gray-500 underline hover:text-gray-900 disabled:opacity-50"
                >
                  Use automatic choice
                </button>
              )}
              {message && (
                <span role="status" className={`text-sm ${message.kind === 'ok' ? 'text-green-700' : 'text-red-700'}`}>
                  {message.text}
                </span>
              )}
            </div>

            <p className="text-xs text-gray-500">
              {ai.source === 'saved'
                ? 'Currently used: the platform and model you saved.'
                : 'Nothing saved yet: the first platform that has an API key is used automatically.'}
            </p>
          </div>
        )}

        <div className="relative mt-6 rounded-2xl bg-white/70 p-4">
          <h3 className="text-sm font-semibold text-gray-900">Used for</h3>
          <ul className="mt-2 space-y-1.5 text-sm text-gray-600">
            {USED_FOR.map((line) => (
              <li key={line} className="flex gap-2">
                <span className="mt-1.5 inline-block h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: ACCENT.to }} />
                {line}
              </li>
            ))}
          </ul>
        </div>
      </section>
    </div>
  )
}
