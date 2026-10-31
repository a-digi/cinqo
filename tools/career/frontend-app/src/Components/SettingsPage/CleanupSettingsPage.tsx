import { useEffect, useState } from 'react'
import {
  fetchCleanupSettings,
  previewCleanup,
  runCleanup,
  saveCleanupSettings,
  type CleanupPreview,
  type CleanupSettings,
} from '../../api'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { ACCENTS } from '../JobsPage/jobsTheme'
import {
  automaticText,
  cleanupRequest,
  formErrors,
  isChanged,
  keptLines,
  largeRunWarning,
  lastRunText,
  previewHeadline,
  requestKey,
  toForm,
  toSettings,
  type CleanupForm,
} from './cleanupRules'

const ACCENT = ACCENTS[3] // rose: this page deletes things

const KEPT_ALWAYS = [
  'Jobs on any profile’s job board, whatever their status',
  'Jobs with a CV, or a CV being made',
  'Jobs whose match is still running',
  'Jobs you added by hand (unless you tick “Also delete jobs I added by hand” above)',
]

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function NumberField(props: { label: string; value: string; error?: string; unit: string; onChange: (v: string) => void }) {
  return (
    <label className="block text-sm">
      <span className="text-gray-700">{props.label}</span>
      <span className="mt-1 flex items-center gap-2">
        <input
          type="text"
          inputMode="numeric"
          value={props.value}
          onChange={(e) => {
            props.onChange(e.target.value)
          }}
          aria-invalid={props.error !== undefined}
          className={`w-24 rounded-lg border px-3 py-1.5 text-sm ${props.error ? 'border-red-400' : 'border-gray-300'}`}
        />
        <span className="text-gray-500">{props.unit}</span>
      </span>
      {props.error && <span className="mt-1 block text-xs text-red-700">{props.error}</span>}
    </label>
  )
}

function Check(props: { label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        type="checkbox"
        checked={props.checked}
        onChange={(e) => {
          props.onChange(e.target.checked)
        }}
        className="mt-1"
      />
      <span>
        <span className="font-medium text-gray-900">{props.label}</span>
        {props.hint && <span className="block text-xs text-gray-500">{props.hint}</span>}
      </span>
    </label>
  )
}

// Settings → Job cleanup (step 131): delete jobs added more than N days ago, and jobs that no profile matches well,
// by hand (preview first, then delete) or on a schedule. Nothing here ever runs without a preview you can see; the
// automatic run waits a full interval after each save.
export function CleanupSettingsPage() {
  const [saved, setSaved] = useState<CleanupSettings | null>(null)
  const [form, setForm] = useState<CleanupForm | null>(null)
  const [loadError, setLoadError] = useState('')
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)
  // A preview is kept with the request that made it: change the form and it is stale, and deleting is not offered.
  const [preview, setPreview] = useState<{ key: string; data: CleanupPreview } | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [running, setRunning] = useState(false)
  const [runMessage, setRunMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null)

  useEffect(() => {
    fetchCleanupSettings()
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

  const current: CleanupForm = form // the narrowed form, for the closures below
  const errors = formErrors(form)
  const request = cleanupRequest(form)
  const key = requestKey(request)
  const freshPreview = preview !== null && preview.key === key ? preview.data : null
  const changed = isChanged(form, saved)

  function edit(patch: Partial<CleanupForm>) {
    setForm((f) => (f ? { ...f, ...patch } : f))
    setMessage(null)
    setConfirming(false)
    setRunMessage(null)
  }

  function save() {
    const settings = toSettings(current)
    if (!settings) return
    setSaving(true)
    setMessage(null)
    saveCleanupSettings(settings)
      .then((s) => {
        setSaved(s)
        setForm(toForm(s))
        setMessage({ kind: 'ok', text: 'Saved.' })
      })
      .catch((err: unknown) => {
        setMessage({ kind: 'error', text: errorText(err) })
      })
      .finally(() => {
        setSaving(false)
      })
  }

  function showPreview() {
    if (!request) return
    setPreviewing(true)
    setConfirming(false)
    setRunMessage(null)
    previewCleanup(request)
      .then((data) => {
        setPreview({ key, data })
      })
      .catch((err: unknown) => {
        setPreview(null)
        setRunMessage({ kind: 'error', text: errorText(err) })
      })
      .finally(() => {
        setPreviewing(false)
      })
  }

  function deleteNow() {
    if (!request || !freshPreview) return
    setRunning(true)
    runCleanup(request)
      .then((res) => {
        setConfirming(false)
        setPreview(null)
        setRunMessage({ kind: 'ok', text: `Deleted ${String(res.deleted)} ${res.deleted === 1 ? 'job' : 'jobs'}.` })
        return fetchCleanupSettings().then(setSaved)
      })
      .catch((err: unknown) => {
        setConfirming(false)
        setRunMessage({ kind: 'error', text: errorText(err) })
      })
      .finally(() => {
        setRunning(false)
      })
  }

  const warning = freshPreview ? largeRunWarning(freshPreview) : ''
  const hint = automaticText(form)

  return (
    <div className="career-v-spin career-a-sheen career-t-type max-w-3xl p-6 font-sans text-gray-900">
      <h1 className="text-xl font-semibold">Job cleanup</h1>
      <p className="mt-1 mb-5 text-sm text-gray-500">Delete old jobs and jobs that match none of your profiles, by hand or on a schedule.</p>

      <section
        className="career-enter career-delay-1 rounded-3xl border p-6 shadow-sm"
        style={{ borderColor: ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${ACCENT.soft} 100%)` }}
      >
        <h2 className="text-base font-semibold">Rules</h2>
        <p className="text-sm text-gray-500">A job is deleted when either switched-on rule picks it. The preview below shows exactly which.</p>

        <div className="mt-4 space-y-5">
          <div className="space-y-2">
            <Check
              label="Delete jobs that are old"
              hint="Counted from the day a crawl last saw the job (a crawl that finds it again makes it new again)."
              checked={form.olderEnabled}
              onChange={(v) => {
                edit({ olderEnabled: v })
              }}
            />
            {form.olderEnabled && (
              <div className="ml-6">
                <NumberField
                  label="Older than"
                  unit="days"
                  value={form.olderThanDays}
                  error={errors.olderThanDays}
                  onChange={(v) => {
                    edit({ olderThanDays: v })
                  }}
                />
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Check
              label="Delete jobs that match no profile well"
              hint="Every profile that can judge the job must give it a manual match score under the limit. Jobs no profile can judge are kept."
              checked={form.lowMatchEnabled}
              onChange={(v) => {
                edit({ lowMatchEnabled: v })
              }}
            />
            {form.lowMatchEnabled && (
              <div className="ml-6">
                <NumberField
                  label="Under"
                  unit="% for every profile"
                  value={form.minMatchPercent}
                  error={errors.minMatchPercent}
                  onChange={(v) => {
                    edit({ minMatchPercent: v })
                  }}
                />
              </div>
            )}
          </div>

          <Check
            label="Also delete jobs I added by hand"
            hint="Off by default: a job you typed in yourself is kept."
            checked={form.includeManual}
            onChange={(v) => {
              edit({ includeManual: v })
            }}
          />
        </div>

        <div className="mt-6 rounded-2xl bg-white/70 p-4">
          <h3 className="text-sm font-semibold text-gray-900">Never deleted</h3>
          <ul className="mt-2 space-y-1.5 text-sm text-gray-600">
            {KEPT_ALWAYS.map((line) => (
              <li key={line} className="flex gap-2">
                <span className="mt-1.5 inline-block h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: ACCENT.to }} />
                {line}
              </li>
            ))}
          </ul>
        </div>

        <h2 className="mt-6 text-base font-semibold">Automatic cleanup</h2>
        <div className="mt-3 space-y-3">
          <Check
            label="Run the cleanup by itself"
            hint="Uses the rules above. Off until you switch it on."
            checked={form.enabled}
            onChange={(v) => {
              edit({ enabled: v })
            }}
          />
          {form.enabled && (
            <div className="ml-6">
              <NumberField
                label="Every"
                unit="hours"
                value={form.intervalHours}
                error={errors.intervalHours}
                onChange={(v) => {
                  edit({ intervalHours: v })
                }}
              />
            </div>
          )}
          {hint && <p className="ml-6 text-xs text-gray-500">{hint}</p>}
        </div>

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
        <p className="mt-3 text-xs text-gray-500">{lastRunText(saved)}</p>
      </section>

      <section className="career-enter career-delay-2 mt-5 rounded-3xl border border-gray-200 p-6 shadow-sm">
        <h2 className="text-base font-semibold">Clean up now</h2>
        <p className="text-sm text-gray-500">Preview what the rules above would delete, with the numbers as they are in the form (saved or not).</p>

        <div className="mt-4 flex flex-wrap items-center gap-3">
          <button
            type="button"
            disabled={previewing || running || request === null}
            onClick={showPreview}
            className="rounded-full border border-gray-300 px-5 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50 disabled:opacity-50"
          >
            {previewing ? 'Checking…' : 'Preview'}
          </button>
          {request === null && (
            <span className="text-xs text-gray-500">
              {errors.olderThanDays || errors.minMatchPercent ? 'Fix the numbers above first.' : 'Switch on at least one rule.'}
            </span>
          )}
        </div>

        {preview !== null && freshPreview === null && (
          <p className="mt-4 text-sm text-amber-700">The form changed since this preview, so it is out of date. Preview again.</p>
        )}

        {freshPreview && (
          <div className="mt-4 space-y-3">
            <p className="text-sm font-medium text-gray-900">{previewHeadline(freshPreview)}</p>
            {warning && <InfoBox variant="warning" label="Large cleanup" message={warning} />}
            {keptLines(freshPreview).length > 0 && (
              <ul className="space-y-1 text-xs text-gray-500">
                {keptLines(freshPreview).map((line) => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
            )}
            {freshPreview.sample.length > 0 && (
              <div>
                <p className="mb-1 text-xs font-medium text-gray-700">
                  {freshPreview.wouldDelete > freshPreview.sample.length
                    ? `The first ${String(freshPreview.sample.length)}:`
                    : 'These jobs:'}
                </p>
                <ul className="divide-y divide-gray-100 rounded-xl border border-gray-200 text-sm">
                  {freshPreview.sample.map((s) => (
                    <li key={s.id} className="flex items-baseline justify-between gap-3 px-3 py-1.5">
                      <span className="min-w-0 truncate">
                        <span className="font-medium text-gray-900">{s.title}</span>
                        {s.company && <span className="text-gray-500"> · {s.company}</span>}
                      </span>
                      <span className="shrink-0 text-xs text-gray-500">
                        {s.bestPercent === undefined ? `added ${s.crawledAt.slice(0, 10)}` : `best match ${String(s.bestPercent)} %`}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {freshPreview.wouldDelete > 0 && !confirming && (
              <button
                type="button"
                disabled={running}
                onClick={() => {
                  setConfirming(true)
                }}
                className="rounded-full px-5 py-2 text-sm font-medium text-white shadow-sm disabled:opacity-50"
                style={{ background: ACCENT.to }}
              >
                Delete {String(freshPreview.wouldDelete)} {freshPreview.wouldDelete === 1 ? 'job' : 'jobs'}…
              </button>
            )}
            {confirming && (
              <div role="alertdialog" aria-label="Confirm deleting" className="rounded-xl border border-red-200 bg-red-50 p-3">
                <p className="text-sm text-red-800">
                  Delete {String(freshPreview.wouldDelete)} {freshPreview.wouldDelete === 1 ? 'job' : 'jobs'} for good? This cannot be undone.
                </p>
                <div className="mt-2 flex gap-3">
                  <button
                    type="button"
                    disabled={running}
                    onClick={deleteNow}
                    className="rounded-full bg-red-600 px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
                  >
                    {running ? 'Deleting…' : 'Yes, delete'}
                  </button>
                  <button
                    type="button"
                    disabled={running}
                    onClick={() => {
                      setConfirming(false)
                    }}
                    className="text-sm text-gray-600 underline disabled:opacity-50"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {runMessage && (
          <p role="status" className={`mt-4 text-sm ${runMessage.kind === 'ok' ? 'text-green-700' : 'text-red-700'}`}>
            {runMessage.text}
          </p>
        )}
      </section>
    </div>
  )
}
