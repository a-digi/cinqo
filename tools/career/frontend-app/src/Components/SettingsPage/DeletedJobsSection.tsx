import { useEffect, useState } from 'react'
import { allowAllDeletedJobs, allowDeletedJob, fetchDeletedJobs, type DeletedJobsList } from '../../api'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { deletedAtText, listHeadline, reasonLabel, rowDetail, scoreText } from './deletedJobsRules'

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// Settings → Job cleanup → "Deleted jobs" (step 133): the jobs the crawler will not add again, each with Allow again.
// Jobs you delete, and jobs the low-match cleanup deletes, are remembered by the address of their posting; jobs removed
// only because they are old are not (a repost comes back).
export function DeletedJobsSection() {
  const [list, setList] = useState<DeletedJobsList | null>(null)
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState<number | null>(null)
  const [confirmAll, setConfirmAll] = useState(false)
  const [message, setMessage] = useState('')

  function load() {
    fetchDeletedJobs()
      .then((l) => {
        setList(l)
        setError('')
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
  }

  useEffect(load, [])

  function allow(id: number) {
    setBusyId(id)
    setMessage('')
    allowDeletedJob(id)
      .then(() => {
        setMessage('Allowed again: the next crawl may add it.')
        load()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setBusyId(null)
      })
  }

  function allowAll() {
    setConfirmAll(false)
    setMessage('')
    allowAllDeletedJobs()
      .then((n) => {
        setMessage(`${String(n)} ${n === 1 ? 'job' : 'jobs'} allowed again: the next crawl may add them.`)
        load()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
  }

  return (
    <section className="career-enter career-delay-3 mt-5 rounded-3xl border border-gray-200 p-6 shadow-sm">
      <h2 className="text-base font-semibold">Deleted jobs</h2>
      <p className="text-sm text-gray-500">
        Jobs you delete, and jobs the low-match cleanup deletes, are remembered so the crawler does not add them again. Jobs removed only because
        they are old are not: if they come back they are genuine reposts.
      </p>

      {error && (
        <div className="mt-3">
          <InfoBox variant="error" label="Problem" message={error} />
        </div>
      )}
      {list === null && !error && <p className="mt-3 text-sm text-gray-500">Loading…</p>}

      {list !== null && (
        <div className="mt-3 space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-gray-700">{listHeadline(list)}</p>
            {list.available && list.total > 0 && (
              <button
                type="button"
                onClick={() => {
                  setConfirmAll(true)
                }}
                className="text-sm text-gray-600 underline hover:text-gray-900"
              >
                Allow all again…
              </button>
            )}
          </div>
          {message && (
            <p role="status" className="text-sm text-green-700">
              {message}
            </p>
          )}
          {list.items.length > 0 && (
            <ul className="divide-y divide-gray-100 rounded-xl border border-gray-200 text-sm">
              {list.items.map((j) => (
                <li key={j.id} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate font-medium text-gray-900" title={j.sourceUrl}>
                      {j.title}
                    </p>
                    <p className="truncate text-xs text-gray-500">{rowDetail(j)}</p>
                    <p className="text-xs text-gray-400">
                      {reasonLabel(j.reason)} · {deletedAtText(j.deletedAt)}
                      {scoreText(j.matchScore) !== '' && ` · ${scoreText(j.matchScore)}`}
                    </p>
                  </div>
                  <button
                    type="button"
                    disabled={busyId === j.id}
                    onClick={() => {
                      allow(j.id)
                    }}
                    className="shrink-0 rounded-full border border-gray-300 px-4 py-1.5 text-xs font-medium text-gray-800 hover:bg-gray-50 disabled:opacity-50"
                  >
                    Allow again
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <ConfirmationModal
        open={confirmAll}
        title="Allow all deleted jobs again?"
        message={`The crawler may then add ${list ? String(list.total) : 'all'} of them again, and the low-match cleanup may delete them again. Nothing is brought back right now.`}
        confirmLabel="Allow all"
        onConfirm={allowAll}
        onCancel={() => {
          setConfirmAll(false)
        }}
      />
    </section>
  )
}
