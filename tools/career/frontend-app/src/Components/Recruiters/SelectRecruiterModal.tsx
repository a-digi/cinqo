import { useEffect, useMemo, useState } from 'react'
import { fetchRecruiters, mapJobRecruiter, type JobRecruiter, type Recruiter } from '../../api'
import { NEW_RECRUITER_PATH } from '../../paths'
import { Modal } from '../../Shared/Modal/Modal'
import { buildChoices, filterChoices, modalState, pickable } from './recruiterMapRules'

interface SelectRecruiterModalProps {
  // The job to map a recruiter to; the modal is open while this is set. companyId (when the job is linked to a company) puts that
  // company's recruiters first.
  job: { id: string; title: string; company?: string; companyId?: string } | null
  // The recruiters already mapped to the job: shown, but not pickable again.
  mapped: readonly JobRecruiter[]
  onClose: () => void
  // Called with the job's recruiters after one was mapped; the modal closes itself.
  onMapped: (recruiters: JobRecruiter[]) => void
}

// "Map to recruiter" (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 3): pick one of the recruiters (all of them, the ones
// who work for the job's own company first, searchable by name, e-mail and company) and map it to the job. One pick per visit.
export function SelectRecruiterModal({ job, mapped, onClose, onMapped }: SelectRecruiterModalProps) {
  const [recruiters, setRecruiters] = useState<Recruiter[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const jobId = job?.id ?? null
  useEffect(() => {
    if (jobId === null) return
    setQuery('')
    setSelectedId(null)
    setError('')
    setLoading(true)
    fetchRecruiters()
      .then(setRecruiters)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [jobId])

  const mappedIds = useMemo(() => mapped.map((m) => m.id), [mapped])
  const choices = useMemo(() => buildChoices(recruiters, mappedIds, job?.companyId), [recruiters, mappedIds, job?.companyId])
  const visible = filterChoices(choices, query)
  const state = modalState(choices, visible, query)
  const picked = pickable(choices, selectedId)

  function map() {
    if (!job || !picked) return
    setSaving(true)
    setError('')
    mapJobRecruiter(job.id, picked.id)
      .then((answer) => {
        onMapped(answer.recruiters)
        onClose()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  return (
    <Modal open={job !== null} title="Map to recruiter" onClose={onClose}>
      {job && (
        <p className="mb-3 text-xs text-gray-500">
          For <span className="font-medium text-gray-700">{job.title}</span>
          {job.company ? ` at ${job.company}` : ''}
        </p>
      )}
      {loading && <p className="text-sm text-gray-500">Loading…</p>}
      {error && <p className="mb-2 text-sm text-red-700">{error}</p>}
      {!loading && !error && state === 'no-recruiters' && (
        <p className="text-sm text-gray-500">
          No recruiters yet.{' '}
          <a
            href={NEW_RECRUITER_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(NEW_RECRUITER_PATH)
            }}
            className="text-blue-700 underline hover:text-blue-800"
          >
            Add a recruiter
          </a>
        </p>
      )}
      {!loading && state === 'all-mapped' && <p className="mb-2 text-sm text-gray-500">Every recruiter is already mapped to this job.</p>}
      {!loading && (state === 'list' || state === 'no-match' || state === 'all-mapped') && (
        <>
          <input
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
            }}
            placeholder="Search by name, e-mail or company…"
            aria-label="Search recruiters"
            className="mb-2 w-full rounded-md border border-gray-200 px-3 py-1.5 text-sm focus:border-gray-400 focus:outline-none"
          />
          {state === 'no-match' && <p className="py-2 text-sm text-gray-400">No recruiter matches your search.</p>}
          <ul className="flex max-h-72 flex-col gap-1.5 overflow-y-auto pr-1">
            {visible.map((c) => (
              <li key={c.id}>
                <label
                  className={`flex items-start gap-2.5 rounded-md border px-3 py-2 text-sm ${
                    c.mapped
                      ? 'cursor-not-allowed border-gray-100 bg-gray-50 text-gray-400'
                      : selectedId === c.id
                        ? 'cursor-pointer border-gray-900 bg-gray-50'
                        : 'cursor-pointer border-gray-200 hover:bg-gray-50'
                  }`}
                >
                  <input
                    type="radio"
                    name="recruiter-to-map"
                    checked={selectedId === c.id}
                    disabled={c.mapped}
                    onChange={() => {
                      setSelectedId(c.id)
                    }}
                    className="mt-1"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-gray-900">{c.label}</span>
                    <span className="block truncate text-xs text-gray-500">
                      {[c.companyName, c.email].filter((x) => x !== '').join(' · ')}
                    </span>
                  </span>
                  {c.mapped && <span className="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500">Already mapped</span>}
                  {!c.mapped && c.sameCompany && (
                    <span className="shrink-0 rounded-full bg-blue-50 px-2 py-0.5 text-xs text-blue-700">Same company</span>
                  )}
                </label>
              </li>
            ))}
          </ul>
        </>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={map}
          disabled={!picked || saving}
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
        >
          {saving ? 'Mapping…' : 'Map'}
        </button>
      </div>
    </Modal>
  )
}
