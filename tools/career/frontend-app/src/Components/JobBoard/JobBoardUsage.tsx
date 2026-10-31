import { useCallback, useEffect, useState } from 'react'
import { fetchJobBoardUsage, type JobBoardUsageEntry } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { jobBoardPathFor } from '../../paths'
import { AddToBoardModal } from './AddToBoardModal'
import { columnAccent } from './jobBoardRules'

// "Profiles using this job" (step 126), on the job's page: every profile whose job board has the job and the job's
// status on it (the status links to that board), and "Add to board" for the others. See
// plan/ai/tools/career/step-126-job-board-statuses.md.
export function JobBoardUsage({ jobId, jobTitle }: { jobId: string; jobTitle: string }) {
  const [usage, setUsage] = useState<JobBoardUsageEntry[] | null>(null)
  const [error, setError] = useState('')
  const [adding, setAdding] = useState(false)

  const load = useCallback(() => {
    setError('')
    fetchJobBoardUsage(jobId)
      .then(setUsage)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [jobId])

  useEffect(() => {
    load()
  }, [load])

  return (
    <div className="career-enter career-delay-2 w-full shrink-0 rounded-md border border-gray-200 bg-white p-5 shadow-sm md:w-64">
      <h2 className="mb-3 text-sm font-semibold text-gray-900">
        Profiles using this job{usage !== null && usage.length > 0 ? ` (${String(usage.length)})` : ''}
      </h2>
      {error !== '' && <p className="mb-2 text-xs text-red-700">{error}</p>}
      {usage === null ? (
        error === '' && <p className="text-xs text-gray-500">Loading…</p>
      ) : usage.length === 0 ? (
        <p className="text-xs text-gray-500">No profile has this job on its job board yet.</p>
      ) : (
        <ul className="space-y-2">
          {usage.map((u) => {
            const accent = columnAccent(u.kind, u.statusName)
            const name = `${u.firstName} ${u.lastName}`.trim() || 'Profile'
            return (
              <li key={u.profileId} className="flex items-center justify-between gap-2 text-sm">
                <span className="min-w-0 truncate text-gray-800">{name}</span>
                <AppLink
                  href={jobBoardPathFor(u.profileId)}
                  className="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold hover:underline"
                  style={{ background: accent.soft, color: accent.text }}
                >
                  {u.statusName}
                </AppLink>
              </li>
            )
          })}
        </ul>
      )}
      <button
        type="button"
        onClick={() => {
          setAdding(true)
        }}
        className="mt-4 w-full rounded-md border border-gray-200 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50"
      >
        Add to board
      </button>
      <AddToBoardModal
        job={adding ? { id: jobId, title: jobTitle } : null}
        onClose={() => {
          setAdding(false)
        }}
        onAdded={load}
      />
    </div>
  )
}
