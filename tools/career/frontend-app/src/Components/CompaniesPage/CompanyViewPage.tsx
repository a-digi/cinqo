import { useEffect, useState } from 'react'
import { fetchCompanies, fetchJobs, type Company, type Job } from '../../api'
import { BuildingIcon } from '../../Shared/Icons/icons'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { COMPANIES_PATH, JOB_DETAILS_PATH, RECRUITERS_PATH } from '../../paths'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { plural } from './companyStats'

// How many of the company's jobs the page asks for at once; a company with more
// shows the first ones and a note pointing at the Jobs page, which has paging.
const JOB_LIMIT = 100

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString()
}

// The page behind a company's "View" button: the company's details and the jobs
// linked to it. Read-only and menu-less, like the job details page. The company
// comes from the existing list route and the jobs from the Jobs route's own
// ?companyId= filter, so it needs no backend of its own.
export function CompanyViewPage() {
  const companyId = new URLSearchParams(window.location.search).get('id') ?? ''
  const [company, setCompany] = useState<Company | null>(null)
  const [jobs, setJobs] = useState<Job[]>([])
  const [total, setTotal] = useState(0)
  const [isLoaded, setIsLoaded] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!companyId) {
      setError('No company selected')
      setIsLoaded(true)
      return
    }
    Promise.all([fetchCompanies(), fetchJobs('', '', [companyId], undefined, JOB_LIMIT, 0)])
      .then(([companies, result]) => {
        const found = companies.find((c) => c.id === companyId)
        if (!found) {
          setError('Company not found')
          return
        }
        setCompany(found)
        setJobs(result.jobs)
        setTotal(result.total)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [companyId])

  function go(e: React.MouseEvent, path: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(path)
  }

  const backLink = (
    <a
      href={COMPANIES_PATH}
      onClick={(e) => {
        go(e, COMPANIES_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All companies
    </a>
  )

  if (!company) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{isLoaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  const accent = accentFor(company.name)
  const matched = jobs.filter((j) => j.matchStatus === 'completed' && j.matchScore !== undefined).length

  return (
    <div className="max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<BuildingIcon />}
        title={company.name}
        description={company.description || 'No description yet.'}
        pill={{ text: plural(company.jobCount, 'linked job', 'linked jobs') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <div className="mb-3 text-sm font-semibold text-gray-700">Linked jobs</div>
          {error && <div className="mb-2 text-sm text-red-700">{error}</div>}
          {jobs.length === 0 && (
            <div
              className="rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No jobs are linked to this company yet. Link one from the Jobs page.
            </div>
          )}
          <div className="career-stagger space-y-3">
            {jobs.map((job) => (
              <div
                key={job.id}
                className="flex items-center gap-3 rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
              >
                <span
                  className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                  style={{ background: accent.to }}
                  aria-hidden="true"
                >
                  {initialOf(job.title)}
                </span>
                <div className="min-w-0 flex-1">
                  <a
                    href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`}
                    onClick={(e) => {
                      go(e, `${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`)
                    }}
                    className="block truncate text-sm font-semibold text-blue-700 hover:underline"
                  >
                    {job.title}
                  </a>
                  <div className="mt-0.5 truncate text-xs text-gray-500">
                    {[job.location, job.portalName, formatDate(job.postedAt || job.crawledAt)].filter(Boolean).join(' · ')}
                  </div>
                </div>
                {job.matchStatus === 'completed' && job.matchScore !== undefined && (
                  <div className="w-32 shrink-0">
                    <MatchScoreBar score={job.matchScore} skills={job.matchedSkills} jobTitle={job.title} />
                  </div>
                )}
              </div>
            ))}
          </div>
          {total > jobs.length && (
            <div className="mt-3 text-xs text-gray-500">
              Showing {jobs.length} of {total} jobs.{' '}
              <a
                href={`/tools/career/jobs?companyId=${encodeURIComponent(company.id)}`}
                onClick={(e) => {
                  go(e, `/tools/career/jobs?companyId=${encodeURIComponent(company.id)}`)
                }}
                className="font-medium text-blue-700 underline"
              >
                See all on the Jobs page
              </a>
            </div>
          )}
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-56">
          <SummaryRail
            title="Company"
            segments={[
              { value: matched, color: '#16a34a' },
              { value: jobs.length - matched, color: '#cbd5e1' },
            ]}
            centerValue={String(company.jobCount)}
            centerLabel={company.jobCount === 1 ? 'job' : 'jobs'}
            rows={[
              { label: 'Linked jobs', value: String(company.jobCount), color: ACCENTS[0].to },
              { label: 'Matched', value: String(matched), color: '#16a34a' },
              { label: 'Recruiters', value: String(company.recruiterCount), color: ACCENTS[4].to },
              { label: 'Added', value: formatDate(company.createdAt) || '–', color: ACCENTS[6].to },
            ]}
          >
            <a
              href={`${RECRUITERS_PATH}?companyId=${encodeURIComponent(company.id)}`}
              onClick={(e) => {
                go(e, `${RECRUITERS_PATH}?companyId=${encodeURIComponent(company.id)}`)
              }}
              className="mt-4 inline-block text-xs font-medium text-blue-700 hover:underline"
            >
              {plural(company.recruiterCount, 'recruiter', 'recruiters')} →
            </a>
          </SummaryRail>
        </aside>
      </div>
    </div>
  )
}
