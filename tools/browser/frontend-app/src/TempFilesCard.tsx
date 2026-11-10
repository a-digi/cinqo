import { useState } from 'react'
import { clearTempFiles, type TempFilesCleared, type TempFilesResponse } from './api'
import { FolderIcon, TrashIcon } from './icons'
import { CountUp } from './Shared/Motion/CountUp'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { PAGE_ACCENT } from './Shared/theme'
import { canDelete, clearedText, deleteQuestion, EXPLANATION, formatBytes, KEPT_NOTE, partText, visibleParts } from './tempFilesRules'
import { useTempFiles } from './useTempFiles'

// "Temporary files" (plan 248): the size of the whole temporary files folder of the tool, what is in it, and a button that deletes what is not in use. The
// tool clears the folder by itself at every start and stop and when a new page copy is made; this card shows it and lets a person clear it now. It reads
// GET /temp-files when the page opens and every few seconds while the page is visible, and is left out entirely by a backend that does not have the route.
export function TempFilesCard() {
  const { state, refresh } = useTempFiles()
  if (state.kind === 'loading' || state.kind === 'missing') return null
  if (state.kind === 'error') {
    return (
      <section className={`${cardClassName} mb-5`} style={cardStyle} aria-label="Temporary files">
        <Heading />
        <p role="alert" className="mt-3 text-sm text-red-700">
          {state.message}
        </p>
      </section>
    )
  }
  return <Ready data={state.data} onChanged={refresh} />
}

function Heading() {
  return (
    <div className="flex items-center gap-3">
      <span
        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl text-white"
        style={{ background: `linear-gradient(135deg, ${PAGE_ACCENT.from}, ${PAGE_ACCENT.to})` }}
      >
        <FolderIcon />
      </span>
      <h2 className="text-base font-semibold text-gray-900">Temporary files</h2>
    </div>
  )
}

function Ready({ data, onChanged }: { data: TempFilesResponse; onChanged: () => void }) {
  const [asking, setAsking] = useState(false)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<TempFilesCleared | null>(null)
  const [error, setError] = useState('')
  const deletable = canDelete(data)

  function remove() {
    setBusy(true)
    setError('')
    clearTempFiles()
      .then((r) => {
        setResult(r)
        setAsking(false)
        onChanged()
      })
      .catch((e: unknown) => {
        setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  return (
    <section className={`${cardClassName} mb-5`} style={cardStyle} aria-label="Temporary files">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Heading />
        <div className="text-right" data-testid="temp-files-size">
          <div className="text-2xl font-semibold tabular-nums text-gray-900">
            <CountUp value={formatBytes(data.bytes)} durationMs={900} />
          </div>
          <div className="text-xs text-gray-500">in the whole folder</div>
        </div>
      </div>

      <ul className="mt-4 divide-y divide-gray-100 rounded-2xl border border-gray-100 bg-white" aria-label="What the folder holds">
        {visibleParts(data.parts).map((p) => (
          <li key={p.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 text-sm">
            <span className="min-w-0 flex-1 font-medium text-gray-900">{p.label}</span>
            <span className="text-xs text-gray-600">{partText(p)}</span>
          </li>
        ))}
      </ul>

      <p className="mt-3 text-xs text-gray-500">
        {EXPLANATION} {KEPT_NOTE}
      </p>

      <div className="mt-4 flex flex-wrap items-center gap-3 border-t border-gray-100 pt-3">
        {asking ? (
          <div
            role="alertdialog"
            aria-label="Confirm the delete"
            className="flex flex-wrap items-center gap-3 rounded-2xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900"
          >
            <span>{deleteQuestion(data.bytes)}</span>
            <button
              type="button"
              disabled={busy}
              className="inline-flex items-center gap-1.5 rounded-xl bg-red-600 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
              onClick={remove}
            >
              <TrashIcon className="h-3.5 w-3.5" />
              {busy ? 'Deleting…' : 'Delete'}
            </button>
            <button
              type="button"
              disabled={busy}
              className="rounded-xl border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 transition hover:bg-gray-50 disabled:opacity-50"
              onClick={() => {
                setAsking(false)
              }}
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            type="button"
            disabled={!deletable}
            title={deletable ? undefined : 'Nothing to delete: the folder only holds the list of running browsers.'}
            className="inline-flex items-center gap-1.5 rounded-xl border border-red-200 bg-white px-3 py-1.5 text-xs font-semibold text-red-700 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => {
              setResult(null)
              setAsking(true)
            }}
          >
            <TrashIcon className="h-3.5 w-3.5" />
            Delete temporary files
          </button>
        )}
        {result && (
          <p role="status" className="text-xs text-gray-700">
            {clearedText(result)}
          </p>
        )}
        {error && (
          <p role="alert" className="text-xs text-red-700">
            {error}
          </p>
        )}
      </div>
    </section>
  )
}
