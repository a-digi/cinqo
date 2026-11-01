import { useEffect, useState } from 'react'
import { fetchCrawlerSettings, saveCrawlerSettings } from '../../api'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { ACCENTS } from '../JobsPage/jobsTheme'
import { Check, NumberField } from './SettingsFields'
import { IgnoredJobsSection } from './IgnoredJobsSection'
import {
  RISK_NOTE,
  formErrors,
  isChanged,
  savedNotice,
  saveFailureText,
  summaryLine,
  toForm,
  toSettings,
  type CrawlerForm,
  type CrawlerSettings,
} from './crawlerRules'

const ACCENT = ACCENTS[5] // teal: the crawler

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// Settings → Job crawler (step 150): "Ignore jobs". When it is on, the crawler asks every rule about each NEW job before it inserts it,
// and a job a rule refuses is not inserted (it is listed under "Kept out" instead). The first rule: insert a job only if at least one
// profile matches it with the limit or more. Jobs that are already saved are never touched. Off until you switch it on.
export function CrawlerSettingsPage() {
  const [saved, setSaved] = useState<CrawlerSettings | null>(null)
  const [form, setForm] = useState<CrawlerForm | null>(null)
  const [loadError, setLoadError] = useState('')
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  useEffect(() => {
    fetchCrawlerSettings()
      .then((s) => {
        setSaved(s)
        setForm(toForm(s))
      })
      .catch((err: unknown) => {
        setLoadError(errorText(err))
      })
  }, [])

  if (loadError) {
    return (
      <div className="max-w-3xl p-6 font-sans">
        <InfoBox variant="error" label="Could not load" message={loadError} />
      </div>
    )
  }
  if (!saved || !form) return <div className="p-6 font-sans text-sm text-gray-500">Loading…</div>

  const current: CrawlerForm = form // the narrowed form, for the closures below
  const errors = formErrors(form)
  const changed = isChanged(form, saved)

  function edit(patch: Partial<CrawlerForm>) {
    setForm((f) => (f ? { ...f, ...patch } : f))
    setMessage(null)
  }

  function save() {
    const settings = toSettings(current)
    if (!settings) return
    setSaving(true)
    setMessage(null)
    saveCrawlerSettings(settings)
      .then((s) => {
        setSaved(s)
        setForm(toForm(s))
        setMessage({ kind: 'ok', text: savedNotice(s) })
      })
      .catch((err: unknown) => {
        setMessage({ kind: 'error', text: saveFailureText(errorText(err)) })
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <div className="career-v-spin career-a-sheen career-t-type max-w-3xl p-6 font-sans text-gray-900">
      <h1 className="text-xl font-semibold">Job crawler</h1>
      <p className="mt-1 mb-5 text-sm text-gray-500">Settings for the jobs the crawler finds.</p>

      <section
        className="career-enter career-delay-1 rounded-3xl border p-6 shadow-sm"
        style={{ borderColor: ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${ACCENT.soft} 100%)` }}
      >
        <h2 className="text-base font-semibold">Ignore jobs</h2>
        <p className="text-sm text-gray-500">
          Keep jobs you do not want out of your job list: the crawler checks each new job against your rules before it saves it.
        </p>

        <div className="mt-4 space-y-4">
          <Check
            label="Ignore jobs"
            hint="Off until you switch it on. Jobs that are already saved are never removed or hidden by these rules."
            checked={form.ignoreEnabled}
            onChange={(v) => {
              edit({ ignoreEnabled: v })
            }}
          />

          {form.ignoreEnabled && (
            <div className="ml-6 space-y-4 border-l-2 pl-4" style={{ borderColor: ACCENT.from }}>
              <div className="space-y-2">
                <Check
                  label="Ignore jobs that match no profile well"
                  hint="A job is saved only if at least one of your profiles matches it with the limit or more. A job no profile can judge (no profile, no persona, too little to compare) is saved."
                  checked={form.minMatchEnabled}
                  onChange={(v) => {
                    edit({ minMatchEnabled: v })
                  }}
                />
                {form.minMatchEnabled && (
                  <div className="ml-6">
                    <NumberField
                      label="At least one profile must match with at least"
                      unit="%"
                      value={form.minMatchPercent}
                      error={errors.minMatchPercent}
                      onChange={(v) => {
                        edit({ minMatchPercent: v })
                      }}
                    />
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        <p className="mt-4 text-sm text-gray-700">{summaryLine(saved)}</p>

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <button
            type="button"
            disabled={saving || !changed || toSettings(form) === null}
            onClick={save}
            className="rounded-full px-5 py-2 text-sm font-medium text-white shadow-sm disabled:opacity-50"
            style={{ background: ACCENT.to }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          {message && (
            <span role="status" className={`text-sm ${message.kind === 'ok' ? 'text-green-700' : 'text-red-700'}`}>
              {message.text}
            </span>
          )}
        </div>

        <div className="mt-6 rounded-2xl bg-white/70 p-4">
          <h3 className="text-sm font-semibold text-gray-900">Good to know</h3>
          <p className="mt-1 text-sm text-gray-600">{RISK_NOTE}</p>
        </div>
      </section>

      <IgnoredJobsSection ignoreEnabled={saved.ignoreEnabled} />
    </div>
  )
}
