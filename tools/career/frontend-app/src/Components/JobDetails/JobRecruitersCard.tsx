import { useEffect, useState } from 'react'
import { fetchJobRecruiters, unmapJobRecruiter, type JobRecruiter } from '../../api'
import { UsersIcon } from '../../Shared/Icons/icons'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { viewRecruiterPathFor } from '../../paths'
import { SelectRecruiterModal } from '../Recruiters/SelectRecruiterModal'
import { InfoCard } from './InfoCard'
import { mailtoHref, recruiterRow, withoutRecruiter } from './jobRecruitersRules'

interface JobRecruitersCardProps {
  job: { id: string; title: string; company?: string; companyId?: string }
}

// The "Recruiters" card on a job's page (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 5): the recruiters mapped to the job,
// each with a "Remove" (an unmapping needs no confirmation: it is a link, redone in one click), and the button that opens the same
// modal as the Jobs list to map another one.
export function JobRecruitersCard({ job }: JobRecruitersCardProps) {
  const [recruiters, setRecruiters] = useState<JobRecruiter[] | null>(null)
  const [error, setError] = useState('')
  const [picking, setPicking] = useState(false)
  const [removing, setRemoving] = useState<string | null>(null)

  const jobId = job.id
  useEffect(() => {
    setRecruiters(null)
    setError('')
    fetchJobRecruiters(jobId)
      .then((list) => {
        setRecruiters(list ?? [])
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [jobId])

  function remove(recruiterId: string) {
    setError('')
    setRemoving(recruiterId)
    unmapJobRecruiter(jobId, recruiterId)
      .then(() => {
        setRecruiters((prev) => (prev ? withoutRecruiter(prev, recruiterId) : prev))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setRemoving(null)
      })
  }

  return (
    <InfoCard look="recruiters" icon={<UsersIcon className="h-4 w-4" />} title="Recruiters">
      {error !== '' && (
        <p role="alert" className="mb-3 text-xs text-red-700">
          {error}
        </p>
      )}
      {recruiters === null && error === '' && <p className="mb-3 text-xs text-gray-500">Loading…</p>}
      {recruiters !== null && recruiters.length === 0 && <p className="mb-3 text-xs text-gray-500">No recruiter mapped yet.</p>}
      {recruiters !== null && recruiters.length > 0 && (
        <ul className="mb-3 flex flex-col gap-1.5">
          {recruiters.map((r) => {
            const row = recruiterRow(r)
            const mailto = mailtoHref(r.email)
            return (
              <li
                key={r.id}
                className="flex items-start justify-between gap-2 rounded-md border border-white bg-white/80 px-3 py-2 text-sm"
              >
                <div className="min-w-0">
                  <AppLink
                    href={viewRecruiterPathFor(r.id)}
                    className="block truncate font-medium text-gray-900 hover:text-blue-700 hover:underline"
                  >
                    {row.title}
                  </AppLink>
                  {row.subtitle !== '' && <p className="truncate text-xs text-gray-500">{row.subtitle}</p>}
                  {mailto !== null && (
                    <a href={mailto} className="text-xs text-blue-700 underline hover:text-blue-800">
                      Send an e-mail
                    </a>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => {
                    remove(r.id)
                  }}
                  disabled={removing === r.id}
                  aria-label={`Remove ${row.title} from this job`}
                  className="shrink-0 text-xs text-gray-500 underline hover:text-gray-700 disabled:opacity-50"
                >
                  {removing === r.id ? 'Removing…' : 'Remove'}
                </button>
              </li>
            )
          })}
        </ul>
      )}
      <button
        type="button"
        onClick={() => {
          setPicking(true)
        }}
        disabled={recruiters === null}
        className="rounded-md border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
      >
        Map to recruiter
      </button>
      <SelectRecruiterModal
        job={picking ? job : null}
        mapped={recruiters ?? []}
        onClose={() => {
          setPicking(false)
        }}
        onMapped={setRecruiters}
      />
    </InfoCard>
  )
}
