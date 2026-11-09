import { useId } from 'react'
import { accents } from '../../../Shared/Components/CardArt/accents'
import { IconBadge } from '../../../Shared/Components/CardArt/icons'
import { Decor } from '../../../Shared/Components/CardArt/shapes'
import { formatBytes } from '../../../Shared/Components/Download/format'
import type { Download } from '../../../api/downloads'
import { ActiveDownload, InstallRow } from './InstallRow'
import type {
  LocalAiEngine,
  LocalAiHardware,
  LocalAiModel,
  LocalAiRuntime,
  LocalAiSettings,
  LocalAiStatus,
  LocalAiSuggestion,
  LocalAiTestResult,
} from './localAiApi'
import {
  addedByText,
  addedSourceLine,
  canRemoveModel,
  suggestionFacts,
  suggestedByText,
  suggestionWhyLines,
  contextAdvice,
  contextFits,
  contextLabel,
  engineLine,
  fitKind,
  fitLabel,
  hardwareSummary,
  idleHelp,
  modelFacts,
  setupView,
  supportText,
  switchVisual,
  testSummary,
  toolsLine,
  type ContextAdvice,
} from './localAiFormat'
import { contextOptions, draftAfterPicking, hasProblems, switchState, type Draft, type DraftProblems } from './localAiRules'

const accent = accents.teal

// What to say about the chosen context size for a model that can use tools, with a one-click way to take the recommended size.
function ContextAdviceLine({ advice, disabled, onTake }: { advice: ContextAdvice; disabled: boolean; onTake: (size: number) => void }) {
  if (advice.kind === 'none') return null
  const tone = advice.kind === 'ok' ? 'text-teal-800' : 'text-amber-800'
  return (
    <p className={`mt-2 text-xs ${tone}`}>
      {advice.text}
      {advice.kind === 'low' && advice.size !== null && (
        <>
          {' '}
          <button
            type="button"
            disabled={disabled}
            onClick={() => {
              onTake(advice.size ?? 0)
            }}
            className="font-semibold underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500 disabled:opacity-50"
          >
            Use {advice.size.toLocaleString('en-US')} tokens
          </button>
        </>
      )}
    </p>
  )
}

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
  engine,
  testing,
  testResult,
  testError,
  onTest,
  downloadedFiles,
  downloading,
  failures,
  installBusy,
  installError,
  onStartInstall,
  onCancelDownload,
  onDownloadFinished,
  onRemoveDownloads,
  suggestions,
  onConfirmSuggestion,
  onDismissSuggestion,
  onRemoveModel,
  onToggle,
  onContinueSetup,
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
  engine: LocalAiEngine
  // The test: running, its last result, or why it could not be run at all.
  testing: boolean
  testResult: LocalAiTestResult | null
  testError: string
  onTest: () => void
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
  // Models tools suggested that wait for a decision (plan 220): confirming downloads and adds one, dismissing hides it for good; an added model can be removed.
  suggestions: LocalAiSuggestion[]
  onConfirmSuggestion: (s: LocalAiSuggestion) => void
  onDismissSuggestion: (s: LocalAiSuggestion) => void
  onRemoveModel: (m: LocalAiModel) => void
  // Switching on or off: the page saves it at once, so switching on starts the setup in the background (plan 205).
  onToggle: (enabled: boolean) => void
  // Continue or try again: starts the downloads that are missing.
  onContinueSetup: () => void
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
  const ctxId = useId()
  const idleId = useId()
  const switchNoteId = useId()
  const groupId = useId()
  const locked = !canManage || saving
  const sw = switchState(draft, catalog)
  const switchOff = locked || sw.locked
  const chosen = catalog.find((m) => m.id === draft.modelId)
  const canTest = canManage && chosen?.enable.ok === true && !testing
  const visual = switchVisual(saved, status)
  const on = visual === 'on'
  const setup = setupView(saved, status)

  return (
    <form
      className="space-y-5"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        onSave()
      }}
    >
      <Card label="Local Agents" title="Use a local model" badge={<StatusBadge on={on} />}>
        <div className="flex items-start gap-3">
          <button
            type="button"
            role="switch"
            aria-checked={draft.enabled}
            aria-labelledby={switchLabelId}
            aria-describedby={switchNoteId}
            disabled={switchOff}
            onClick={() => {
              onToggle(!draft.enabled)
            }}
            className="relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500 focus-visible:ring-offset-2 disabled:opacity-50"
            style={{
              background: !draft.enabled
                ? '#d1d5db'
                : visual === 'pending'
                  ? '#f59e0b'
                  : `linear-gradient(135deg, ${accent.from}, ${accent.to})`,
            }}
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
              Off by default. While off, &quot;Local AI&quot; is not offered anywhere. When you switch it on, Cinqo first downloads what it
              needs in the background (the program that runs the models and the chosen model, if they are not on this computer yet); once
              that is done it shows up as a platform when a new conversation is created. Nothing you write is sent to a cloud provider by
              it.
            </p>
            {sw.locked && (
              <p className="mt-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
                It cannot be switched on yet. {sw.reason}
              </p>
            )}
            {setup.kind !== 'none' && (
              <div
                role="status"
                data-testid="local-ai-setup"
                className={`mt-3 rounded-lg border px-3 py-2 text-xs ${
                  setup.kind === 'failed'
                    ? 'border-red-200 bg-red-50 text-red-900'
                    : setup.kind === 'ready'
                      ? 'border-teal-200 bg-teal-50 text-teal-900'
                      : 'border-amber-200 bg-amber-50 text-amber-900'
                }`}
              >
                <p className="font-semibold">{setup.title}</p>
                <p className="mt-0.5">{setup.detail}</p>
                {status.state === 'preparing' && status.preparing !== null && status.preparing.downloadId !== '' && (
                  <div className="mt-2">
                    <ActiveDownload
                      key={status.preparing.downloadId}
                      id={status.preparing.downloadId}
                      busy={installBusy}
                      onCancel={() => {
                        onCancelDownload(status.preparing?.downloadId ?? '')
                      }}
                      onFinished={(d) => {
                        onDownloadFinished(status.preparing?.step === 'program' ? 'runtime' : saved.modelId, d)
                      }}
                    />
                  </div>
                )}
                {canManage && setup.actions.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-2">
                    {(setup.actions.includes('retry') || setup.actions.includes('continue')) && (
                      <button
                        type="button"
                        disabled={installBusy || saving}
                        onClick={onContinueSetup}
                        className="rounded-full border border-current px-3 py-1 text-xs font-semibold hover:bg-white/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500 disabled:opacity-50"
                      >
                        {setup.actions.includes('retry') ? 'Try again' : 'Continue'}
                      </button>
                    )}
                    {setup.actions.includes('switch_off') && (
                      <button
                        type="button"
                        disabled={installBusy || saving}
                        onClick={() => {
                          onToggle(false)
                        }}
                        className="rounded-full border border-gray-300 bg-white px-3 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500 disabled:opacity-50"
                      >
                        Switch off
                      </button>
                    )}
                  </div>
                )}
              </div>
            )}
            <p className={`mt-2 text-xs ${engine.state === 'failed' ? 'text-red-700' : 'text-gray-500'}`}>
              <span className="font-medium">The model program: </span>
              {engineLine(engine)}
            </p>
          </div>
        </div>
      </Card>

      {suggestions.length > 0 && (
        <Card label="Suggested models" title="Suggested by tools">
          <p className="mb-3 text-xs text-gray-500">
            Tools, and Cinqo itself, suggested these models. Nothing is downloaded until you say so. Cinqo checks the file it downloads, but
            it has not tested the model.
          </p>
          <ul className="space-y-3">
            {suggestions.map((s) => (
              <li key={s.id} className="rounded-2xl border border-gray-200 bg-white/70 p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-sm font-semibold text-gray-900">{s.name}</p>
                  <FitBadge fits={s.fits} />
                </div>
                <p className="mt-1 text-xs text-gray-600">{suggestionFacts(s)}</p>
                <p className="mt-1 text-xs font-medium text-violet-700">{suggestedByText(s)}</p>
                {suggestionWhyLines(s).map((line, i) => (
                  <p key={i} className="mt-1 text-xs text-gray-700">
                    {line}
                  </p>
                ))}
                <p className="mt-1 text-[11px] text-gray-400">
                  Hugging Face {s.repo} · published by {s.publisher}
                  {s.shipped === true && ' · ships with Cinqo'}
                </p>
                {canManage && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      type="button"
                      disabled={installBusy}
                      onClick={() => {
                        onConfirmSuggestion(s)
                      }}
                      className="rounded-full border border-teal-600 px-3 py-1 text-xs font-semibold text-teal-700 transition hover:bg-teal-50 disabled:opacity-50"
                    >
                      Download ({formatBytes(s.sizeBytes)})
                    </button>
                    <button
                      type="button"
                      disabled={installBusy}
                      onClick={() => {
                        onDismissSuggestion(s)
                      }}
                      className="rounded-full border border-gray-300 px-3 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                    >
                      Dismiss
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card label="Model" title="Default model">
        <fieldset aria-describedby={`${groupId}-note`}>
          <legend className="sr-only">Default model</legend>
          <p id={`${groupId}-note`} className="mb-3 text-xs text-gray-500">
            Cinqo downloads the model you choose and runs it for you. This is the default model: it is the one used when you switch the
            local AI on and the one a tool such as Career preselects. Tools can choose any other model you have downloaded, so you do not
            have to come back here to switch. The fit is a rule of thumb: the file plus a quarter for the working memory, plus 3 GiB for the
            system and Cinqo.
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
                      onChange(draftAfterPicking(draft, m, saved.modelId))
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
                      {addedByText(m) !== '' && (
                        <span className="rounded-full bg-violet-50 px-2 py-0.5 text-[11px] font-semibold text-violet-700">
                          {addedByText(m)}
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-gray-600">{modelFacts(m)}</p>
                    <p className="mt-1 text-xs text-gray-500">{m.note}</p>
                    <p className="mt-1 text-xs text-gray-500">{toolsLine(m)}</p>
                    <p className="mt-1 text-[11px] text-gray-400">
                      License {m.license} · {m.source}
                    </p>
                    {addedSourceLine(m) !== '' && <p className="mt-1 text-[11px] text-gray-500">{addedSourceLine(m)}</p>}
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
                      {canManage && canRemoveModel(m, saved.modelId) && (
                        <button
                          type="button"
                          disabled={installBusy}
                          onClick={() => {
                            onRemoveModel(m)
                          }}
                          className="mt-2 text-xs font-medium text-red-700 hover:text-red-900 disabled:opacity-50"
                        >
                          Remove this model
                        </button>
                      )}
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
                ? `${runtime.name}, ${formatBytes(runtime.sizeBytes)}. Cinqo downloads it when you switch the local AI on, or here.`
                : 'There is no such program for this system yet.'}
            </p>
            <div className="mt-2">
              <InstallRow
                install={runtime.install}
                sizeBytes={runtime.sizeBytes}
                failure={failures.runtime ?? null}
                supported={runtime.supported}
                canManage={canManage}
                busy={installBusy}
                onStart={() => {
                  onStartInstall('runtime', draft.modelId === '' ? undefined : draft.modelId)
                }}
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

      <Card label="Resources" title="Memory and time">
        <div className="space-y-4">
          <div>
            <label htmlFor={ctxId} className="block text-sm font-medium text-gray-900">
              Context size
            </label>
            <select
              id={ctxId}
              value={draft.contextTokens}
              disabled={locked}
              aria-invalid={problems.contextTokens !== undefined}
              aria-describedby={`${ctxId}-note`}
              onChange={(e) => {
                onChange({ ...draft, contextTokens: Number(e.target.value) })
              }}
              className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm focus:border-teal-500 focus:outline-none focus:ring-2 focus:ring-teal-500/30 disabled:bg-gray-50 disabled:text-gray-500 aria-[invalid=true]:border-red-400"
            >
              {contextOptions(draft, catalog).map((n) => {
                const model = catalog.find((m) => m.id === draft.modelId)
                return (
                  <option key={n} value={n} disabled={!contextFits(model, n) && n !== draft.contextTokens}>
                    {contextLabel(n, model)}
                  </option>
                )
              })}
            </select>
            <p id={`${ctxId}-note`} className={`mt-1 text-xs ${problems.contextTokens ? 'text-red-700' : 'text-gray-500'}`}>
              {problems.contextTokens ??
                'How much of a conversation the model reads at once. A larger context lets it remember more and uses more memory. A conversation that grows past it must be started again.'}
            </p>
            <ContextAdviceLine
              advice={contextAdvice(
                draft.contextTokens,
                catalog.find((m) => m.id === draft.modelId),
              )}
              disabled={locked}
              onTake={(size) => {
                onChange({ ...draft, contextTokens: size })
              }}
            />
          </div>
          <div>
            <label htmlFor={idleId} className="block text-sm font-medium text-gray-900">
              Unload after (minutes)
            </label>
            <input
              id={idleId}
              type="number"
              min={0}
              max={1440}
              step={1}
              inputMode="numeric"
              value={Number.isNaN(draft.idleMinutes) ? '' : draft.idleMinutes}
              disabled={locked}
              aria-invalid={problems.idleMinutes !== undefined}
              aria-describedby={`${idleId}-note`}
              onChange={(e) => {
                onChange({ ...draft, idleMinutes: e.target.valueAsNumber })
              }}
              className="mt-1 w-40 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm focus:border-teal-500 focus:outline-none focus:ring-2 focus:ring-teal-500/30 disabled:bg-gray-50 disabled:text-gray-500 aria-[invalid=true]:border-red-400"
            />
            <p id={`${idleId}-note`} className={`mt-1 text-xs ${problems.idleMinutes ? 'text-red-700' : 'text-gray-500'}`}>
              {problems.idleMinutes ?? idleHelp(draft.idleMinutes)}
            </p>
          </div>
        </div>
      </Card>

      <Card label="Test" title="Test the model">
        <p className="text-xs text-gray-500">
          Sends one tiny question to the chosen model and times the answer. If the model is not loaded yet, it is loaded first, which can
          take a moment.
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <button
            type="button"
            disabled={!canTest}
            onClick={onTest}
            className="rounded-full border border-teal-600 px-4 py-1.5 text-xs font-semibold text-teal-700 transition hover:bg-teal-50 disabled:opacity-50"
          >
            {testing ? 'Testing…' : 'Test the model'}
          </button>
          {!chosen && <span className="text-xs text-gray-500">Choose a model first.</span>}
          {chosen && !chosen.enable.ok && (
            <span className="text-xs text-gray-500">
              {chosen.enable.reason === '' ? 'This model cannot be run yet.' : `${chosen.enable.reason}.`}
            </span>
          )}
        </div>
        <div role="status" aria-live="polite" className="mt-3 text-sm">
          {testing && <p className="text-gray-600">Waiting for the model… {engine.state === 'starting' ? 'It is loading.' : ''}</p>}
          {!testing && testError && <p className="text-red-700">{testError}</p>}
          {!testing && !testError && testResult && (
            <p className={testResult.ok ? 'text-teal-800' : 'text-red-700'}>{testSummary(testResult)}</p>
          )}
        </div>
      </Card>

      <Card label="This computer" title="This computer">
        <p className="text-sm text-gray-900">{hardwareSummary(hardware)}</p>
        <p className="mt-1 text-xs text-gray-500">{supportText(hardware)}</p>
      </Card>

      <Card label="What to expect" title="What to expect">
        <ul className="list-disc space-y-1 pl-5 text-xs text-gray-600">
          <li>Small local models are slower and less capable than the cloud models.</li>
          <li>
            There are no sub-agents. A model that can use tools gets only the few a task names (the CV import, for example); the others only
            answer.
          </li>
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
