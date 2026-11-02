import { useEffect, useState } from 'react'
import { fetchRecruiterJobs, fetchRecruiters, unmapJobRecruiter, type Recruiter, type RecruiterJob } from '../../api'
import { UsersIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { JOB_DETAILS_PATH, RECRUITERS_PATH, VIEW_COMPANY_PATH } from '../../paths'
import { mailtoHref } from '../JobDetails/jobRecruitersRules'
import { accentFor, initialOf } from '../JobsPage/jobsTheme'
import { companyNamesText, recruiterLabel } from '../Recruiters/recruiterMapRules'
import { formatDay, jobMeta, mappedJobsPill, withoutJob } from './recruiterViewRules'

// The page behind a recruiter's "View" link (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 6), in the manner of the profile and
// company view pages: a header card, the recruiter's details and the jobs they are mapped to, each with "Remove from this job". The
// recruiter and the company come from the existing list routes, the jobs from the mapping route.
export function RecruiterViewPage() {
  const recruiterId = new URLSearchParams(window.location.search).get('id') ?? ''
  const [recruiter, setRecruiter] = useState<Recruiter | null>(null)
  const [jobs, setJobs] = useState<RecruiterJob[]>([])
  const [isLoaded, setIsLoaded] = useState(false)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [removing, setRemoving] = useState<string | null>(null)

  useEffect(() => {
    if (!recruiterId) {
      setError('No recruiter selected')
      setIsLoaded(true)
      return
    }
    Promise.all([fetchRecruiters(), fetchRecruiterJobs(recruiterId)])
      .then(([recruiters, mapped]) => {
        const found = recruiters.find((r) => r.id === recruiterId)
        if (!found || mapped === null) {
          setError('Recruiter not found')
          return
        }
        setRecruiter(found)
        setJobs(mapped)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [recruiterId])

  function removeJob(jobId: string) {
    setActionError('')
    setRemoving(jobId)
    unmapJobRecruiter(jobId, recruiterId)
      .then(() => {
        setJobs((prev) => withoutJob(prev, jobId))
      })
      .catch((err: unknown) => {
        setActionError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setRemoving(null)
      })
  }

  const backLink = (
    <AppLink href={RECRUITERS_PATH} className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline">
      ← All recruiters
    </AppLink>
  )

  if (!recruiter) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{isLoaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  const name = recruiterLabel(recruiter)
  const mailto = mailtoHref(recruiter.email)
  const companyNames = companyNamesText(recruiter.companies)

  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<UsersIcon />}
        title={name}
        description={companyNames !== '' ? `Recruiter at ${companyNames}.` : 'Recruiter.'}
        pill={{ text: mappedJobsPill(jobs.length) }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <section aria-label="Overview" className={`${cardClassName} w-full shrink-0 lg:w-72`} style={cardStyle}>
          <h2 className="mb-3 text-sm font-semibold text-gray-700">Overview</h2>
          <dl className="space-y-2 text-sm">
            <div>
              <dt className="text-xs text-gray-500">E-mail</dt>
              <dd>
                {mailto !== null ? (
                  <a href={mailto} className="break-all text-blue-700 underline hover:text-blue-800">
                    {recruiter.email.trim()}
                  </a>
                ) : (
                  <span className="text-gray-400">No e-mail stored</span>
                )}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-gray-500">{recruiter.companies.length === 1 ? 'Company' : 'Companies'}</dt>
              <dd>
                {recruiter.companies.length > 0 ? (
                  <ul className="space-y-0.5">
                    {recruiter.companies.map((c) => (
                      <li key={c.id}>
                        <AppLink href={`${VIEW_COMPANY_PATH}?id=${encodeURIComponent(c.id)}`} className="text-blue-700 hover:underline">
                          {c.name}
                        </AppLink>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <span className="text-gray-400">No company</span>
                )}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-gray-500">Added</dt>
              <dd>{formatDay(recruiter.createdAt) || '–'}</dd>
            </div>
          </dl>
        </section>

        <section aria-label="Mapped jobs" className="min-w-0 flex-1">
          <h2 className="mb-3 text-sm font-semibold text-gray-700">Mapped jobs</h2>
          {actionError !== '' && (
            <p role="alert" className="mb-2 text-sm text-red-700">
              {actionError}
            </p>
          )}
          {jobs.length === 0 && (
            <div className={`${cardClassName} text-center text-sm text-gray-500`} style={cardStyle}>
              No job is mapped to this recruiter yet. Map one from the Jobs list or from a job’s page.
            </div>
          )}
          <div className="career-stagger space-y-3">
            {jobs.map((job) => (
              <div
                key={job.id}
                className={`${cardClassName} flex items-center gap-3 !p-4 transition-shadow hover:shadow-md`}
                style={cardStyle}
              >
                <span
                  className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                  style={{ background: accentFor(job.title).to }}
                  aria-hidden="true"
                >
                  {initialOf(job.title)}
                </span>
                <div className="min-w-0 flex-1">
                  <AppLink
                    href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`}
                    className="block truncate text-sm font-semibold text-blue-700 hover:underline"
                  >
                    {job.title}
                  </AppLink>
                  {jobMeta(job) !== '' && <div className="mt-0.5 truncate text-xs text-gray-500">{jobMeta(job)}</div>}
                  {formatDay(job.mappedAt) !== '' && (
                    <div className="mt-0.5 text-xs text-gray-400">Mapped on {formatDay(job.mappedAt)}</div>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => {
                    removeJob(job.id)
                  }}
                  disabled={removing === job.id}
                  className="shrink-0 rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50 disabled:opacity-50"
                >
                  {removing === job.id ? 'Removing…' : 'Remove from this job'}
                </button>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}
