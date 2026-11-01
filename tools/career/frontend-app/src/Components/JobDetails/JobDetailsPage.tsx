import { useEffect, useState } from 'react'
import { fetchJob, mediaDownloadUrl, type Job } from '../../api'
import { BuildingIcon, ClockIcon, ExternalLinkIcon, GlobeIcon, PDFIcon, TargetIcon } from '../../Shared/Icons/icons'
import { Decor } from '../../Shared/Decor/Decor'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'
import { MatchBreakdown } from '../../Shared/MatchScoreBar/MatchBreakdown'
import { Comments } from '../../Shared/Comments/Comments'
import { COMMENT_TYPE_JOB } from '../../Shared/Comments/commentTypes'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { VIEW_COMPANY_PATH, VIEW_PORTAL_PATH } from '../../paths'
import { JobBoardUsage } from '../JobBoard/JobBoardUsage'
import { PersonaMatchesCard } from './PersonaMatchesCard'
import { InfoCard } from './InfoCard'
import { JobRequirements } from './JobRequirements'

const JOBS_PATH = '/tools/career/jobs'

// One fact about the job (company, platform, place, date) as a small tinted chip in the page's accent.
function FactChip({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <li
      className="flex items-center gap-1.5 rounded-full border px-3 py-1 font-medium"
      style={{ borderColor: PAGE_ACCENT.from, background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
    >
      {icon}
      {children}
    </li>
  )
}

// JobDetailsPage — a real, standalone, bookmarkable page (never a
// modal) showing one job's own full, untruncated details. Reached only
// via the Jobs page's own Eye icon (JobsPage.tsx); has no menu entry
// of its own, same convention CompaniesPage's own "N linked jobs" link
// already established for a filtered-but-menu-less destination.
//
// The id travels as a query parameter (?id=...), not a path segment —
// the host's own tool-routing layer (frontend/src/Components/Tools/
// ToolRouteOutlet.tsx) only ever matches a registered route by an
// EXACT pathname, with no support for dynamic segments, so a fixed
// path plus a query parameter is the only way this tool can give a job
// its own distinct URL. See
// plan/ai/tools/career/step-XX-jobs-page-icon-actions-and-details-page.md.
export function JobDetailsPage() {
  const [job, setJob] = useState<Job | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get('id')
    if (!id) {
      setError('No job id was provided.')
      return
    }
    let cancelled = false
    fetchJob(id)
      .then((fetched) => {
        if (!cancelled) setJob(fetched)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [])

  function handleBack() {
    window.__cinqoToolBridge.navigate(JOBS_PATH)
  }

  return (
    <div className="career-v-spin mx-auto max-w-[1600px] p-6 font-sans text-gray-900">
      <button type="button" onClick={handleBack} className="mb-4 text-sm text-gray-500 underline hover:text-gray-700">
        ← Back to Jobs
      </button>

      {error && <p className="text-sm text-red-700">{error}</p>}

      {!error && !job && <p className="text-sm text-gray-500">Loading…</p>}

      {job && (
        <>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-[7fr_3fr] lg:items-start">
            <div className="min-w-0">
              <div className="career-enter career-delay-1 min-w-0 overflow-hidden rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
                {/* The banner: a soft blue tint with shapes behind the title and the facts, like the dashboard cards. */}
                <div
                  className="relative -m-6 mb-5 overflow-hidden border-b p-6"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 30%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  <Decor shape="diamond" accent={PAGE_ACCENT} />
                  <div className="relative flex items-start justify-between gap-4">
                    <h1 className="text-2xl font-semibold">{job.title}</h1>
                    {job.sourceUrl !== '' && (
                      <a
                        href={job.sourceUrl}
                        target="_blank"
                        rel="noreferrer"
                        title="Open the original posting"
                        className="flex shrink-0 items-center gap-1.5 rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 transition-colors hover:bg-gray-50"
                      >
                        <ExternalLinkIcon />
                        Open original
                      </a>
                    )}
                  </div>

                  <ul className="relative mt-4 flex flex-wrap gap-2 text-sm">
                    {job.company && (
                      <FactChip icon={<BuildingIcon className="h-4 w-4" />}>
                        {/* The company's page, when the job is linked to one. */}
                        {job.companyId ? (
                          <AppLink href={`${VIEW_COMPANY_PATH}?id=${encodeURIComponent(job.companyId)}`} className="hover:underline">
                            {job.company}
                          </AppLink>
                        ) : (
                          job.company
                        )}
                      </FactChip>
                    )}
                    {job.portalName && (
                      <FactChip icon={<GlobeIcon className="h-4 w-4" />}>
                        {/* The platform's page, when it still exists. */}
                        {job.portalId ? (
                          <AppLink href={`${VIEW_PORTAL_PATH}?id=${encodeURIComponent(job.portalId)}`} className="hover:underline">
                            {job.portalName}
                          </AppLink>
                        ) : (
                          job.portalName
                        )}
                      </FactChip>
                    )}
                    {job.location && <FactChip icon={<TargetIcon className="h-4 w-4" />}>{job.location}</FactChip>}
                    {job.postedAt && <FactChip icon={<ClockIcon className="h-4 w-4" />}>{`Posted ${job.postedAt}`}</FactChip>}
                  </ul>
                </div>

                <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-gray-800">
                  {job.description !== undefined && job.description !== '' ? job.description : 'No description available.'}
                </p>
              </div>

              <JobRequirements jobId={job.id} />

              {/* The host draws this panel as a coloured card of its own, so it gets no frame from here. */}
              <Comments entityType={COMMENT_TYPE_JOB} entityId={job.id} />
            </div>

            <div className="flex min-w-0 flex-col gap-4 lg:sticky lg:top-4">
              {job.matchStatus === 'completed' && job.matchScore !== undefined && (
                <InfoCard look="match" icon={<TargetIcon className="h-4 w-4" />} title="Job Match">
                  <MatchScoreBar
                    score={job.matchScore}
                    skills={job.matchedSkills}
                    jobTitle={job.title}
                    kind={job.matchKind}
                    breakdown={job.matchBreakdown}
                  />
                  {job.matchedSkills.length > 0 ? (
                    <div className="mt-4 flex flex-wrap gap-1.5">
                      {job.matchedSkills.map((skill) => (
                        <span key={skill} className="rounded-full border border-white bg-white/80 px-2.5 py-1 text-xs text-gray-700">
                          {skill}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 text-xs text-gray-400">No specific skills matched.</p>
                  )}
                  {job.matchBreakdown && (
                    <div className="mt-4 border-t border-gray-100 pt-3">
                      <p className="mb-2 text-xs font-semibold text-gray-800">How this score was made</p>
                      <MatchBreakdown result={job.matchBreakdown} />
                    </div>
                  )}
                </InfoCard>
              )}

              <PersonaMatchesCard jobId={job.id} />

              <JobBoardUsage jobId={job.id} />

              {job.cvStatus && (
                <InfoCard look="cv" icon={<PDFIcon className="h-4 w-4" />} title="Generated CV">
                  {job.cvStatus === 'completed' && job.cvMediaFileId ? (
                    <a
                      href={mediaDownloadUrl(job.cvMediaFileId)}
                      target="_blank"
                      rel="noreferrer"
                      className="flex items-center gap-1.5 rounded-md border border-green-200 bg-green-50 px-3 py-1.5 text-sm text-green-700 transition-colors hover:bg-green-100"
                    >
                      <PDFIcon className="h-4 w-4" />
                      Download CV
                    </a>
                  ) : job.cvStatus === 'failed' ? (
                    <p className="text-xs text-red-700">{job.cvError ?? 'CV generation failed.'}</p>
                  ) : (
                    <p className="text-xs text-gray-500">Generating…</p>
                  )}
                </InfoCard>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
