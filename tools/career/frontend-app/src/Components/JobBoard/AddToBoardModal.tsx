import { useEffect, useState } from 'react'
import { fetchJobBoardUsage, fetchProfiles, setJobBoardEntry, type JobBoardUsageEntry, type Profile } from '../../api'
import { Modal } from '../../Shared/Modal/Modal'
import { plural } from '../CompaniesPage/companyStats'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { NOT_REVIEWED_ID } from './jobBoardRules'

// "Add to board" (step 126): pick the profiles whose job board should have this job; it starts in Not Reviewed on
// each. A profile that already has the job is shown with its status and cannot be picked again (it is moved on
// that profile's own board). See plan/ai/tools/career/step-126-job-board-statuses.md.
export function AddToBoardModal({
  job,
  onClose,
  onAdded,
}: {
  // The job to add; the modal is open while this is set.
  job: { id: string; title: string } | null
  onClose: () => void
  // Called after profiles were added, with how many, so the page can refresh what it shows.
  onAdded: (count: number) => void
}) {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [usage, setUsage] = useState<JobBoardUsageEntry[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const jobId = job?.id ?? null
  useEffect(() => {
    if (jobId === null) return
    setSelected([])
    setError('')
    setLoading(true)
    Promise.all([fetchProfiles(), fetchJobBoardUsage(jobId)])
      .then(([profileList, usageList]) => {
        setProfiles(profileList)
        setUsage(usageList)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [jobId])

  const statusOf = new Map(usage.map((u) => [u.profileId, u.statusName]))
  const available = profiles.filter((p) => !statusOf.has(p.id))

  function toggle(id: string) {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  function add() {
    if (!job || selected.length === 0) return
    setSaving(true)
    setError('')
    void Promise.allSettled(selected.map((profileId) => setJobBoardEntry(profileId, job.id, NOT_REVIEWED_ID))).then((results) => {
      setSaving(false)
      const done = selected.filter((_, i) => results[i].status === 'fulfilled')
      if (done.length > 0) onAdded(done.length)
      if (done.length === selected.length) {
        onClose()
        return
      }
      // Whatever was added is no longer offered; the rest stays picked, with the reason.
      setUsage((prev) => [
        ...prev,
        ...done.map((profileId) => {
          const p = profiles.find((x) => x.id === profileId)
          return {
            profileId,
            firstName: p?.firstName ?? '',
            lastName: p?.lastName ?? '',
            statusId: NOT_REVIEWED_ID,
            statusName: 'Not Reviewed',
            kind: 'not_reviewed' as const,
          }
        }),
      ])
      setSelected((prev) => prev.filter((id) => !done.includes(id)))
      setError('Some profiles could not be added. Try again.')
    })
  }

  return (
    <Modal open={job !== null} title="Add to job board" onClose={onClose}>
      {job && <p className="mb-3 text-sm text-gray-600">{job.title}</p>}
      {loading ? (
        <p className="text-sm text-gray-500">Loading…</p>
      ) : profiles.length === 0 ? (
        <p className="text-sm text-gray-500">There are no profiles yet. Create a profile first.</p>
      ) : (
        <>
          <p className="mb-2 text-xs text-gray-500">The job starts in Not Reviewed on each board you pick.</p>
          <ul className="mb-4 max-h-72 space-y-1 overflow-y-auto">
            {profiles.map((p) => {
              const status = statusOf.get(p.id)
              const onBoard = status !== undefined
              return (
                <li key={p.id}>
                  <label
                    className={`flex items-center gap-3 rounded-md px-2 py-1.5 text-sm ${onBoard ? 'text-gray-400' : 'cursor-pointer hover:bg-gray-50'}`}
                  >
                    <input
                      type="checkbox"
                      disabled={onBoard}
                      checked={onBoard || selected.includes(p.id)}
                      onChange={() => {
                        toggle(p.id)
                      }}
                    />
                    <span className="min-w-0 flex-1 truncate">{profileLabel(p)}</span>
                    {onBoard && <span className="shrink-0 text-xs">Already on the board · {status}</span>}
                  </label>
                </li>
              )
            })}
          </ul>
          {available.length === 0 && <p className="mb-3 text-xs text-gray-500">This job is already on every profile’s board.</p>}
        </>
      )}
      {error !== '' && (
        <p role="alert" className="mb-3 text-sm text-red-700">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={selected.length === 0 || saving}
          onClick={add}
          className="rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-900 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {selected.length === 0 ? 'Add to board' : `Add to ${plural(selected.length, 'board', 'boards')}`}
        </button>
      </div>
    </Modal>
  )
}
