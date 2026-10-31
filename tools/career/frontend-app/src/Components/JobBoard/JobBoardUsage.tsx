import { useEffect, useState } from 'react'
import {
  fetchJobBoardUsage,
  fetchJobStatuses,
  fetchProfiles,
  removeJobBoardEntry,
  setJobBoardEntry,
  type JobBoardUsageEntry,
  type JobStatusInfo,
  type Profile,
} from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { inputClass } from '../../Shared/inputClass'
import { jobBoardPathFor } from '../../paths'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { columnAccent, statusOptionsFor } from './jobBoardRules'

// The job's status on each profile's job board (step 126), on the job's page: every profile with a status picker.
// Picking a status puts the job on that profile's board in that status (it is added when it is not on the board
// yet, and only changes status when it is); "Not on the board" takes it off again. See
// plan/ai/tools/career/step-126-job-board-statuses.md.
export function JobBoardUsage({ jobId }: { jobId: string }) {
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [statuses, setStatuses] = useState<JobStatusInfo[]>([])
  // The status id the job has on each profile's board; a profile that is missing does not have the job.
  const [current, setCurrent] = useState<ReadonlyMap<string, string>>(new Map())
  const [saving, setSaving] = useState<string | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    Promise.all([fetchProfiles(), fetchJobStatuses(), fetchJobBoardUsage(jobId)])
      .then(([profileList, statusList, usage]) => {
        setProfiles(profileList)
        setStatuses(statusList)
        setCurrent(new Map(usage.map((u: JobBoardUsageEntry) => [u.profileId, u.statusId])))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [jobId])

  function pick(profileId: string, statusId: string) {
    const before = current.get(profileId)
    if ((before ?? '') === statusId) return
    setError('')
    setSaving(profileId)
    setCurrent((prev) => withStatus(prev, profileId, statusId === '' ? undefined : statusId))
    const save = statusId === '' ? removeJobBoardEntry(profileId, jobId) : setJobBoardEntry(profileId, jobId, statusId)
    save
      .catch((err: unknown) => {
        // The change did not save: the picker goes back to what it was.
        setCurrent((prev) => withStatus(prev, profileId, before))
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(null)
      })
  }

  const onBoards = current.size

  return (
    <div className="career-enter career-delay-2 w-full shrink-0 rounded-md border border-gray-200 bg-white p-5 shadow-sm md:w-64">
      <h2 className="mb-1 text-sm font-semibold text-gray-900">Job board{onBoards > 0 ? ` (${String(onBoards)})` : ''}</h2>
      <p className="mb-3 text-xs text-gray-500">Pick a status for a profile to put this job on its board, or to change its status there.</p>
      {error !== '' && (
        <p role="alert" className="mb-2 text-xs text-red-700">
          {error}
        </p>
      )}
      {profiles === null ? (
        error === '' && <p className="text-xs text-gray-500">Loading…</p>
      ) : profiles.length === 0 ? (
        <p className="text-xs text-gray-500">There are no profiles yet. Create a profile first.</p>
      ) : (
        <ul className="space-y-3">
          {profiles.map((p) => {
            const statusId = current.get(p.id)
            const options = statusOptionsFor(statuses, p.id)
            const selected = options.find((o) => o.id === statusId)
            const accent = selected ? columnAccent(selected.kind, selected.name) : null
            return (
              <li key={p.id}>
                <div className="mb-1 flex items-center justify-between gap-2 text-sm">
                  <span className="min-w-0 truncate text-gray-800">{profileLabel(p)}</span>
                  {statusId !== undefined && (
                    <AppLink href={jobBoardPathFor(p.id)} className="shrink-0 text-xs font-medium text-blue-700 hover:underline">
                      Open board
                    </AppLink>
                  )}
                </div>
                <select
                  aria-label={`Status for ${profileLabel(p)}`}
                  value={statusId ?? ''}
                  disabled={saving === p.id}
                  onChange={(e) => {
                    pick(p.id, e.target.value)
                  }}
                  className={`${inputClass} text-xs`}
                  style={accent ? { background: accent.soft, color: accent.text, borderColor: accent.from } : undefined}
                >
                  <option value="">Not on the board</option>
                  {options.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.name}
                    </option>
                  ))}
                </select>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

// The map with one profile's status set, or removed when statusId is undefined.
function withStatus(map: ReadonlyMap<string, string>, profileId: string, statusId: string | undefined): ReadonlyMap<string, string> {
  const next = new Map(map)
  if (statusId === undefined) next.delete(profileId)
  else next.set(profileId, statusId)
  return next
}
