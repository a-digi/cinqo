import { useEffect, useState } from 'react'
import { fetchJob, mediaDownloadUrl, type Job } from '../../api'
import { ExternalLinkIcon, PDFIcon } from '../../Shared/Icons/icons'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'

const JOBS_PATH = '/tools/career/jobs'

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
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <button type="button" onClick={handleBack} className="mb-4 text-sm text-gray-500 underline hover:text-gray-700">
        ← Back to Jobs
      </button>

      {error && <p className="text-sm text-red-700">{error}</p>}

      {!error && !job && <p className="text-sm text-gray-500">Loading…</p>}

      {job && (
        <div className="flex flex-col gap-6 md:flex-row md:items-start">
          <div className="min-w-0 flex-1 rounded-md border border-gray-200 bg-white p-6 shadow-sm">
            <div className="flex items-start justify-between gap-4">
              <h1 className="text-xl font-semibold">{job.title}</h1>
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
            </div>

            <dl className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-sm text-gray-500">
              {job.company && (
                <div>
                  <dt className="inline font-medium text-gray-700">Company: </dt>
                  <dd className="inline">{job.company}</dd>
                </div>
              )}
              {job.portalName && (
                <div>
                  <dt className="inline font-medium text-gray-700">Platform: </dt>
                  <dd className="inline">{job.portalName}</dd>
                </div>
              )}
              {job.location && (
                <div>
                  <dt className="inline font-medium text-gray-700">Location: </dt>
                  <dd className="inline">{job.location}</dd>
                </div>
              )}
              {job.postedAt && (
                <div>
                  <dt className="inline font-medium text-gray-700">Posted: </dt>
                  <dd className="inline">{job.postedAt}</dd>
                </div>
              )}
            </dl>

            <hr className="my-4 border-gray-200" />

            <p className="whitespace-pre-wrap text-sm text-gray-800">{job.description || 'No description available.'}</p>
          </div>

          {job.matchStatus === 'completed' && job.matchScore !== undefined && (
            <div className="w-full shrink-0 rounded-md border border-gray-200 bg-white p-5 shadow-sm md:w-64">
              <h2 className="mb-3 text-sm font-semibold text-gray-900">Job Match</h2>
              <MatchScoreBar score={job.matchScore} skills={job.matchedSkills} jobTitle={job.title} />
              {job.matchedSkills.length > 0 ? (
                <div className="mt-4 flex flex-wrap gap-1.5">
                  {job.matchedSkills.map((skill) => (
                    <span key={skill} className="rounded-full border border-gray-200 bg-gray-100 px-2.5 py-1 text-xs text-gray-700">
                      {skill}
                    </span>
                  ))}
                </div>
              ) : (
                <p className="mt-4 text-xs text-gray-400">No specific skills matched.</p>
              )}
            </div>
          )}

          {job.cvStatus && (
            <div className="w-full shrink-0 rounded-md border border-gray-200 bg-white p-5 shadow-sm md:w-64">
              <h2 className="mb-3 text-sm font-semibold text-gray-900">Generated CV</h2>
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
            </div>
          )}
        </div>
      )}
    </div>
  )
}
