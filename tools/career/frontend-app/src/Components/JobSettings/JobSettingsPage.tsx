import { useEffect, useRef, useState } from 'react'
import { fetchModelCatalog, startModelDownload, selectModel, removeModel, type SemanticModelCatalogEntry } from '../../api'
import { DownloadIcon, TrashIcon } from '../../Shared/Icons/icons'

// JobSettingsPage lets a user download one of a small, hardcoded
// catalog of pretrained word-vector models (backend's own
// semantic_model.go) and pick one as "active" for the deterministic
// "Match now" feature's own semantic fallback — never bundled into the
// Go binary, fetched into the backend's own data directory on demand
// instead. Polls GET /jobs/model/catalog, same "plain, stateless list
// refresh" convention CrawlMonitorPage already established (there is
// no local optimistic state to reconcile — every row here already IS
// the server's own live view). See
// plan/ai/tools/career/step-XX-semantic-match-models.md.
const MODEL_SETTINGS_POLL_INTERVAL_MS = 3000

export function JobSettingsPage() {
  const [entries, setEntries] = useState<SemanticModelCatalogEntry[]>([])
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState<string | null>(null)

  function load() {
    fetchModelCatalog()
      .then((fetched) => {
        setEntries(fetched)
        setError('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to load model catalog.')
      })
      .finally(() => {
        setLoaded(true)
      })
  }

  const pollTokenRef = useRef(0)
  useEffect(() => {
    const myToken = pollTokenRef.current + 1
    pollTokenRef.current = myToken
    load()
    const interval = setInterval(() => {
      if (pollTokenRef.current === myToken) load()
    }, MODEL_SETTINGS_POLL_INTERVAL_MS)
    return () => {
      pollTokenRef.current += 1
      clearInterval(interval)
    }
  }, [])

  function handleDownload(id: string) {
    setBusyId(id)
    startModelDownload(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to start download.')
      })
      .finally(() => {
        setBusyId(null)
      })
  }

  function handleSelect(id: string) {
    setBusyId(id)
    selectModel(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to set active model.')
      })
      .finally(() => {
        setBusyId(null)
      })
  }

  function handleRemove(id: string) {
    setBusyId(id)
    removeModel(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Failed to remove model.')
      })
      .finally(() => {
        setBusyId(null)
      })
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Job Settings</h1>
      <p className="mb-5 text-sm text-gray-500">
        Semantic match models — an optional upgrade to "Match now" that lets it recognize skills by meaning, not just exact wording.
      </p>

      <div className="mb-6 rounded-md border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
        <h2 className="mb-2 font-medium text-gray-900">How semantic matching models work</h2>
        <ol className="list-decimal space-y-1.5 pl-4">
          <li>
            <span className="font-medium text-gray-900">Download</span> — fetches the model's source archive and extracts only the file for
            this option, discarding the rest. The extracted file is cached on this server's disk so it doesn't need to be downloaded again.
          </li>
          <li>
            <span className="font-medium text-gray-900">Set active</span> — marks one downloaded model as the one "Match now" will use. Only
            one model can be active at a time; switching is instant if the new one is already downloaded.
          </li>
          <li>
            <span className="font-medium text-gray-900">Using it in a match</span> — when you run "Match now" on a job, the active model is
            loaded into memory just for that scoring, in addition to the exact keyword matching used previously. It's kept in memory for 5
            minutes in case you run more matches right after, then automatically unloaded to free up server memory. The file itself stays
            cached on disk the whole time — only memory is freed.
          </li>
          <li>
            <span className="font-medium text-gray-900">Remove</span> — deletes a downloaded model's cached file from disk. If it was the
            active model, matching falls back to keyword-only until you select another.
          </li>
          <li>
            Larger models (bigger vocabulary, higher quality) take longer to download once and longer to load into memory on each first use
            after being idle — smaller models are faster but understand fewer/rarer terms.
          </li>
        </ol>
      </div>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {loaded && (
        <div className="space-y-3">
          {entries.map((entry) => (
            <ModelCard
              key={entry.id}
              entry={entry}
              busy={busyId === entry.id}
              onDownload={() => {
                handleDownload(entry.id)
              }}
              onSelect={() => {
                handleSelect(entry.id)
              }}
              onRemove={() => {
                handleRemove(entry.id)
              }}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ModelCard({
  entry,
  busy,
  onDownload,
  onSelect,
  onRemove,
}: {
  entry: SemanticModelCatalogEntry
  busy: boolean
  onDownload: () => void
  onSelect: () => void
  onRemove: () => void
}) {
  const inProgress = entry.status === 'downloading' || entry.status === 'extracting'

  return (
    <div className="rounded-md border border-gray-200 p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <span className="font-medium">{entry.label}</span>
            {entry.active && <span className="rounded-full bg-gray-900 px-2 py-0.5 text-xs text-white">Active</span>}
          </div>
          <p className="mt-0.5 text-xs text-gray-500">{entry.corpus}</p>
          <p className="mt-1 text-sm text-gray-700">{entry.quality}</p>
          <p className="mt-1 text-xs text-gray-400">
            ~{entry.approxDownloadMb}MB download, ~{entry.approxExtractedMb}MB once extracted
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {entry.status === 'ready' && !entry.active && (
            <button
              type="button"
              onClick={onSelect}
              disabled={busy}
              className="rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Set active
            </button>
          )}
          {(entry.status === 'not_downloaded' || entry.status === 'failed') && (
            <button
              type="button"
              onClick={onDownload}
              disabled={busy}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-2 py-1 text-xs text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <DownloadIcon />
              Download
            </button>
          )}
          {entry.status === 'ready' && (
            <button
              type="button"
              onClick={onRemove}
              disabled={busy}
              title="Delete this model's cached file from disk."
              className="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <TrashIcon />
              Remove
            </button>
          )}
        </div>
      </div>

      {inProgress && (
        <div className="mt-3">
          <div className="mb-1 flex items-center justify-between text-xs text-gray-500">
            <span>{entry.status === 'downloading' ? 'Downloading…' : 'Extracting…'}</span>
            {entry.status === 'downloading' && <span>{entry.progressPercent}%</span>}
          </div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-gray-100">
            <div
              className="h-full rounded-full bg-gray-900 transition-all"
              style={{ width: `${entry.status === 'downloading' ? entry.progressPercent : 100}%` }}
            />
          </div>
        </div>
      )}

      {entry.status === 'failed' && entry.errorMessage && <p className="mt-2 text-xs text-red-700">{entry.errorMessage}</p>}
    </div>
  )
}
