import { useId } from 'react'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { IconBadge } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'
import { formatBytes } from '../../../Shared/Components/Download/format'
import type { Download } from '../../../api/downloads'
import { InstallRow } from './InstallRow'
import type { LocalAiHardware, LocalAiModel, LocalAiRuntime, LocalAiSettings, LocalAiStatus } from './localAiApi'
import { fitKind, fitLabel, hardwareSummary, modelFacts, supportText } from './localAiFormat'
import { hasProblems, switchLocked, type Draft, type DraftProblems } from './localAiRules'

const accent = accents.teal

function Card({ label, title, badge, children }: { label: string; title: string; badge?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section
      aria-label={label}
      className="cinqo-card-enter relative overflow-hidden rounded-3xl border p-5 shadow-sm transition-shadow hover:shadow-lg"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
    >
      <Decor shape="circles" accent={accent} />
      <div className="relative mb-4 flex items-center gap-3">
        <IconBadge name="server" from={accent.from} to={accent.to} />
        <h2 className="min-w-0 flex-1 text-sm font-semibold text-gray-900">{title}</h2>
        {badge}
      </div>
      <div className="relative">{children}</div>
    </section>
  )
}

function StatusBadge({ on }: { on: boolean }) {
  return (
    <span
      className="rounded-full px-2.5 py-0.5 text-xs font-semibold"
      style={on ? { background: accent.soft, color: accent.text } : { background: '#f3f4f6', color: '#6b7280' }}
    >
      {on ? 'On' : 'Off'}
    </span>
  )
}

function FitBadge({ fits }: { fits: boolean | null }) {
  const kind = fitKind(fits)
  const style =
    kind === 'fits'
      ? { background: accent.soft, color: accent.text }
      : kind === 'tight'
        ? { background: '#fffbeb', color: '#b45309' }
        : { background: '#f3f4f6', color: '#6b7280' }
  return (
    <span className="rounded-full px-2 py-0.5 text-[11px] font-semibold" style={style}>
      {fitLabel(fits)}
    </span>
  )
}

// The settings page's form: nothing here talks to the server, the page does. `canManage` is false for someone who may
// only read: every control is then disabled and there is no save button. The local AI is managed by Cinqo only, so the
// form has a switch and a choice of one catalog model, nothing else.
export function LocalAiForm({
  saved,
  status,
  hardware,
  catalog,
  catalogCheckedOn,
  runtime,
  downloadedFiles,
  downloading,
  failures,
  installBusy,
  installError,
  onStartInstall,
  onCancelDownload,
  onDownloadFinished,
  onRemoveDownloads,
  draft,
  problems,
  dirty,
  canManage,
  saving,
  error,
  justSaved,
  onChange,
  onSave,
  onDiscard,
}: {
  saved: LocalAiSettings
  status: LocalAiStatus
  hardware: LocalAiHardware
  catalog: LocalAiModel[]
  catalogCheckedOn: string
  runtime: LocalAiRuntime
  downloadedFiles: boolean
  downloading: boolean
  // The last download of each thing (a model id, or "runtime") that failed: the server forgets finished jobs, the page keeps these.
  failures: Partial<Record<string, Download>>
  installBusy: boolean
  installError: string
  onStartInstall: (key: string, modelId?: string) => void
  onCancelDownload: (downloadId: string) => void
  onDownloadFinished: (key: string, d: Download | null) => void
  onRemoveDownloads: () => void
  draft: Draft
  problems: DraftProblems
  dirty: boolean
  canManage: boolean
  saving: boolean
  error: string
  justSaved: boolean
  onChange: (next: Draft) => void
  onSave: () => void
  onDiscard: () => void
}) {
  const switchLabelId = useId()
  const switchNoteId = useId()
  const groupId = useId()
  const locked = !canManage || saving
  const switchOff = locked || switchLocked(draft, status.canEnable)
  const on = saved.enabled && status.state === 'configured'

  return (
    <form
      className="space-y-5"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        onSave()
      }}
    >
      <Card label="Local AI" title="Use a local model" badge={<StatusBadge on={on} />}>
        <div className="flex items-start gap-3">
          <button
            type="button"
            role="switch"
            aria-checked={draft.enabled}
            aria-labelledby={switchLabelId}
            aria-describedby={switchNoteId}
            disabled={switchOff}
            onClick={() => {
              onChange({ ...draft, enabled: !draft.enabled })
            }}
            className="relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500 focus-visible:ring-offset-2 disabled:opacity-50"
            style={{ background: draft.enabled ? `linear-gradient(135deg, ${accent.from}, ${accent.to})` : '#d1d5db' }}
          >
            <span
              aria-hidden="true"
              className={`inline-block h-5 w-5 rounded-full bg-white shadow transition-transform ${draft.enabled ? 'translate-x-5' : 'translate-x-0.5'}`}
            />
          </button>
          <div className="min-w-0">
            <p id={switchLabelId} className="text-sm font-medium text-gray-900">
              Let conversations use a language model that Cinqo runs on this computer
            </p>
            <p id={switchNoteId} className="mt-1 text-xs text-gray-500">
              Off by default. While off, &quot;Local AI&quot; is not offered anywhere. When it is on, it shows up as a platform when a new
              conversation is created. Nothing you write is sent to a cloud provider by it.
            </p>
            {!status.canEnable && (
              <p className="mt-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
                It cannot be switched on yet: {status.reason}. You can already choose the model below.
              </p>
            )}
          </div>
        </div>
      </Card>

      <Card label="Model" title="Which model">
        <fieldset aria-describedby={`${groupId}-note`}>
          <legend className="sr-only">Model</legend>
          <p id={`${groupId}-note`} className="mb-3 text-xs text-gray-500">
            Cinqo downloads the model you choose and runs it for you. The fit is a rule of thumb: the file plus a quarter for the working
            memory, plus 3 GiB for the system and Cinqo.
          </p>
          <ul className="space-y-3">
            {catalog.map((m) => {
              const selected = draft.modelId === m.id
              return (
                <li
                  key={m.id}
                  className={`relative flex items-start gap-3 rounded-2xl border bg-white/70 p-3 ${selected ? 'border-teal-500 ring-2 ring-teal-500/30' : 'border-gray-200'} ${locked ? 'opacity-70' : ''}`}
                >
                  <input
                    id={`${groupId}-${m.id}`}
                    type="radio"
                    name={`${groupId}-model`}
                    value={m.id}
                    checked={selected}
                    disabled={locked}
                    onChange={() => {
                      onChange({ ...draft, modelId: m.id })
                    }}
                    className="mt-1"
                  />
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <label
                        htmlFor={`${groupId}-${m.id}`}
                        className={`text-sm font-semibold text-gray-900 after:absolute after:inset-0 after:content-[''] ${locked ? 'cursor-not-allowed' : 'cursor-pointer'}`}
                      >
                        {m.name}
                      </label>
                      <span className="rounded-full bg-gray-100 px-2 py-0.5 font-mono text-[11px] text-gray-600">{m.quantization}</span>
                      <FitBadge fits={m.fits} />
                    </div>
                    <p className="mt-1 text-xs text-gray-600">{modelFacts(m)}</p>
                    <p className="mt-1 text-xs text-gray-500">{m.note}</p>
                    <p className="mt-1 text-[11px] text-gray-400">
                      License {m.license} · {m.source}
                    </p>
                    <div className="relative z-10 mt-3">
                      <InstallRow
                        install={m.install}
                        sizeBytes={m.sizeBytes}
                        failure={failures[m.id] ?? null}
                        supported={runtime.supported}
                        canManage={canManage}
                        busy={installBusy}
                        onStart={() => {
                          onStartInstall(m.id, m.id)
                        }}
                        onCancel={onCancelDownload}
                        onFinished={(d) => {
                          onDownloadFinished(m.id, d)
                        }}
                      />
                    </div>
                  </div>
                </li>
              )
            })}
          </ul>
          <div className="mt-4 rounded-2xl border border-gray-200 bg-white/70 p-3">
            <p className="text-sm font-semibold text-gray-900">The program that runs the models</p>
            <p className="mt-1 text-xs text-gray-500">
              {runtime.supported
                ? `${runtime.name}, ${formatBytes(runtime.sizeBytes)}. Cinqo downloads it together with the first model.`
                : 'There is no such program for this system yet.'}
            </p>
            <div className="mt-2">
              <InstallRow
                install={runtime.install}
                sizeBytes={runtime.sizeBytes}
                failure={failures.runtime ?? null}
                supported={runtime.supported}
                canManage={false}
                busy={installBusy}
                onStart={() => undefined}
                onCancel={onCancelDownload}
                onFinished={(d) => {
                  onDownloadFinished('runtime', d)
                }}
              />
            </div>
          </div>
          {installError && (
            <p role="alert" className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
              {installError}
            </p>
          )}
          {canManage && downloadedFiles && (
            <div className="mt-3">
              <button
                type="button"
                disabled={installBusy || downloading || saved.enabled}
                onClick={onRemoveDownloads}
                className="text-xs font-medium text-red-700 hover:text-red-900 disabled:opacity-50"
              >
                Remove all downloaded files
              </button>
              {(saved.enabled || downloading) && (
                <p className="mt-1 text-[11px] text-gray-500">
                  {saved.enabled ? 'Switch the local AI off first.' : 'Cancel the running download first.'}
                </p>
              )}
            </div>
          )}
          {problems.modelId && (
            <p role="alert" className="mt-3 text-xs text-red-700">
              {problems.modelId}
            </p>
          )}
          <p className="mt-3 text-[11px] text-gray-400">
            Sizes, licenses and addresses were checked against the real pages on {catalogCheckedOn}.
          </p>
        </fieldset>
      </Card>

      <Card label="This computer" title="This computer">
        <p className="text-sm text-gray-900">{hardwareSummary(hardware)}</p>
        <p className="mt-1 text-xs text-gray-500">{supportText(hardware)}</p>
      </Card>

      <Card label="What to expect" title="What to expect">
        <ul className="list-disc space-y-1 pl-5 text-xs text-gray-600">
          <li>Small local models are slower and less capable than the cloud models.</li>
          <li>Conversations on the local AI have no tools and no sub-agents: the model only answers.</li>
          <li>It uses the memory and processor of this computer, and the model is a download of a few gigabytes.</li>
          <li>Using it costs nothing per request, and the conversation shows it as free.</li>
        </ul>
      </Card>

      {error && (
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
          {error}
        </p>
      )}

      {canManage ? (
        <div className="flex items-center gap-3">
          <button
            type="submit"
            disabled={!dirty || hasProblems(problems) || saving}
            className="rounded-full px-5 py-2 text-sm font-semibold text-white shadow transition disabled:opacity-50"
            style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          {dirty && !saving && (
            <button type="button" onClick={onDiscard} className="text-sm font-medium text-gray-600 hover:text-gray-900">
              Discard changes
            </button>
          )}
          <span role="status" className="text-xs text-teal-700">
            {justSaved && !dirty ? 'Saved' : ''}
          </span>
        </div>
      ) : (
        <p className="text-xs text-gray-500">You can look at these settings, but changing them needs the platform manage permission.</p>
      )}
    </form>
  )
}
