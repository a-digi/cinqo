import type { Job, MissingDetailState } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { JOB_DETAILS_PATH, VIEW_PLATFORM_PATH } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import type { CompanyCell } from './companyCell'
import { companyText, jobCellLines } from './jobCellLines'
import { accentFor, initialOf } from './jobsTheme'
import { detailsState, isManualJob, platformLabel } from './manualJob'
import { NoDetailsIcon } from './NoDetailsIcon'

// The Job cell of the Jobs table (plan 235): the title with its badges, then under it the company, then under that the platform. One cell, so the
// table has room for the match and the actions; a line with nothing to say is not drawn. The avatar (the company's initial) stays on the left.
export function JobTitleCell({
  job,
  cell,
  missingState,
  onUnlink,
}: {
  job: Job
  // What companyCell(...) made of this job: the text, the page to open, the "linked to" name.
  cell: CompanyCell
  // What is going on with the details that were not crawled, for the "no details" icon.
  missingState: MissingDetailState | undefined
  onUnlink: (jobId: string) => void
}) {
  const platform = platformLabel(job)
  const lines = jobCellLines(cell, platform)
  const details = detailsState(job)
  const look = accentFor(platform)
  const detailsHref = `${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`
  return (
    <div className="flex items-start gap-3">
      <span
        className="mt-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
        style={{ background: accentFor(job.company || job.title).to }}
        aria-hidden="true"
      >
        {initialOf(job.company || job.title)}
      </span>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          {details === 'available' ? (
            <AppLink
              href={detailsHref}
              className="rounded font-medium hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-500"
            >
              {job.title}
            </AppLink>
          ) : (
            <span className="font-medium">{job.title}</span>
          )}
          {details === 'not-crawled' && <NoDetailsIcon state={missingState} platformName={job.portalName} />}
          {job.boardProfileCount > 0 && (
            <a
              href={detailsHref}
              onClick={(e) => {
                e.preventDefault()
                window.__cinqoToolBridge.navigate(detailsHref)
              }}
              title="Profiles whose job board has this job (open the job to see the status on each)"
              className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800 hover:underline"
            >
              On {plural(job.boardProfileCount, 'board', 'boards')}
            </a>
          )}
          {isManualJob(job) && (
            <span
              title="You added this job yourself"
              className="rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-800"
            >
              Manual
            </span>
          )}
        </div>
        {lines.company && (
          <div className="mt-0.5 break-words text-xs text-gray-500">
            {cell.linkTo ? (
              <a
                href={cell.linkTo}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(cell.linkTo ?? '')
                }}
                title="Open this company's page"
                className="text-blue-700 hover:underline focus:outline-none focus-visible:underline"
              >
                {companyText(cell)}
              </a>
            ) : (
              companyText(cell)
            )}
          </div>
        )}
        {lines.companyNote && (
          <div className="mt-0.5 flex items-center gap-1.5 text-xs text-gray-400">
            {cell.linkedName !== null && <>linked to {cell.linkedName}</>}
            <button
              type="button"
              onClick={() => {
                onUnlink(job.id)
              }}
              className="text-gray-400 underline hover:text-red-700"
            >
              unlink
            </button>
          </div>
        )}
        {lines.platform && (
          <div className="mt-1">
            {job.portalId ? (
              <a
                href={`${VIEW_PLATFORM_PATH}?id=${encodeURIComponent(job.portalId)}`}
                onClick={(e) => {
                  e.preventDefault()
                  window.__cinqoToolBridge.navigate(`${VIEW_PLATFORM_PATH}?id=${encodeURIComponent(job.portalId ?? '')}`)
                }}
                title="Open this platform's page"
                className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium hover:underline"
                style={{ background: look.soft, color: look.text }}
              >
                {platform}
              </a>
            ) : (
              <span
                className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                style={{ background: look.soft, color: look.text }}
              >
                {platform}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
