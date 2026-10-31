import { useEffect, useState } from 'react'
import { fetchJobs, fetchPortals, type Job, type Portal } from '../../api'
import { GlobeIcon } from '../../Shared/Icons/icons'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { JOBS_PATH, JOB_DETAILS_PATH, PORTALS_PATH } from '../../paths'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { portalViewNumbers } from './portalView'
import { plural } from './portalStats'

// How many of the portal's jobs the page asks for at once; a portal with more
// shows the first ones and a note pointing at the Jobs page, which has paging.
const JOB_LIMIT = 100

function formatDate(value: string | null | undefined): string {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString()
}

// The page behind a portal: its details, its links and the jobs found through
// them. Read-only and menu-less, like the company page. The portal comes from the
// existing list route and the jobs from the Jobs route's own ?portalId= filter, so
// it needs no backend of its own.
export function PortalViewPage() {
  const portalId = new URLSearchParams(window.location.search).get('id') ?? ''
  const [portal, setPortal] = useState<Portal | null>(null)
  const [jobs, setJobs] = useState<Job[]>([])
  const [total, setTotal] = useState(0)
  const [isLoaded, setIsLoaded] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!portalId) {
      setError('No portal selected')
      setIsLoaded(true)
      return
    }
    Promise.all([fetchPortals(), fetchJobs('', '', undefined, portalId, JOB_LIMIT, 0)])
      .then(([portals, result]) => {
        const found = portals.find((p) => p.id === portalId)
        if (!found) {
          setError('Portal not found')
          return
        }
        setPortal(found)
        setJobs(result.jobs)
        setTotal(result.total)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [portalId])

  function go(e: React.MouseEvent, path: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(path)
  }

  const backLink = (
    <a
      href={PORTALS_PATH}
      onClick={(e) => {
        go(e, PORTALS_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All portals
    </a>
  )

  if (!portal) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{isLoaded ? error : 'Loading…'}</div>
      </div>
    )
  }

  const numbers = portalViewNumbers(portal.links)
  const matched = jobs.filter((j) => j.matchStatus === 'completed' && j.matchScore !== undefined).length
  const jobsPath = `${JOBS_PATH}?portalId=${encodeURIComponent(portal.id)}`

  return (
    <div className="career-v-tilt career-a-float career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<GlobeIcon />}
        title={portal.name}
        description={plural(numbers.links, 'link', 'links')}
        // The pill links on to the Jobs page, filtered to this portal's jobs.
        pill={{ text: plural(total, 'linked job', 'linked jobs'), href: jobsPath }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <div className="mb-3 text-sm font-semibold text-gray-700">Links</div>
          {portal.links.length === 0 && (
            <div
              className="mb-6 rounded-3xl border p-6 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              This portal has no links yet. Add one on the Portals page.
            </div>
          )}
          <div className="mb-6 space-y-2">
            {portal.links.map((link) => {
              const title = link.title?.trim() ?? ''
              return (
                <div key={link.id} className="rounded-2xl border bg-white p-3 text-sm shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
                  <div className="truncate font-medium text-gray-900" title={link.url}>
                    {title === '' ? link.url : title}
                  </div>
                  {title !== '' && (
                    <div className="truncate text-xs text-gray-500" title={link.url}>
                      {link.url}
                    </div>
                  )}
                  <div className="mt-1 text-xs text-gray-500">
                    {[
                      link.lastCrawledAt ? `Last crawled ${formatDate(link.lastCrawledAt)}` : 'Never crawled',
                      link.crawlInstructions?.trim() ? 'Has crawl instructions' : 'No crawl instructions',
                      link.hasActiveCrawlRun ? 'Crawl running now' : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </div>
                </div>
              )
            })}
          </div>

          <div className="mb-3 text-sm font-semibold text-gray-700">Linked jobs</div>
          {error && <div className="mb-2 text-sm text-red-700">{error}</div>}
          {jobs.length === 0 && (
            <div
              className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No jobs were found through this portal yet.
            </div>
          )}
          <div className="career-stagger space-y-3">
            {jobs.map((job) => {
              const accent = accentFor(job.company || job.title)
              const jobPath = `${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`
              return (
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
                      href={jobPath}
                      onClick={(e) => {
                        go(e, jobPath)
                      }}
                      className="block truncate text-sm font-semibold text-blue-700 hover:underline"
                    >
                      {job.title}
                    </a>
                    <div className="mt-0.5 truncate text-xs text-gray-500">
                      {[job.company, job.location, formatDate(job.postedAt || job.crawledAt)].filter(Boolean).join(' · ')}
                    </div>
                  </div>
                  {job.matchStatus === 'completed' && job.matchScore !== undefined && (
                    <div className="w-32 shrink-0">
                      <MatchScoreBar score={job.matchScore} skills={job.matchedSkills} jobTitle={job.title} />
                    </div>
                  )}
                </div>
              )
            })}
          </div>
          {total > jobs.length && (
            <div className="mt-3 text-xs text-gray-500">
              Showing {jobs.length} of {total} jobs.{' '}
              <a
                href={jobsPath}
                onClick={(e) => {
                  go(e, jobsPath)
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
            title="Portal"
            segments={[
              { value: matched, color: '#16a34a' },
              { value: jobs.length - matched, color: '#cbd5e1' },
            ]}
            centerValue={String(total)}
            centerLabel={total === 1 ? 'job' : 'jobs'}
            rows={[
              { label: 'Links', value: String(numbers.links), color: ACCENTS[0].to },
              { label: 'With instructions', value: String(numbers.withInstructions), color: '#16a34a' },
              { label: 'Never crawled', value: String(numbers.neverCrawled), color: ACCENTS[4].to },
              { label: 'Last crawl', value: formatDate(numbers.lastCrawledAt) || '–', color: ACCENTS[6].to },
              { label: 'Added', value: formatDate(portal.createdAt) || '–', color: ACCENTS[2].to },
            ]}
          />
        </aside>
      </div>
    </div>
  )
}
