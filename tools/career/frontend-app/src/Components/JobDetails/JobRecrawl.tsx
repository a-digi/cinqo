import { RefreshIcon } from '../../Shared/Icons/icons'
import type { Job } from '../../api'
import { isRunning, pageNotice, recrawlCheck, recrawlTitle } from '../JobsPage/recrawlRules'
import type { RecrawlControl } from '../JobsPage/useRecrawl'

// The "Recrawl" button in a job page's banner (step 148): crawls this job's page again for a fresh description. Disabled, with the
// reason as its tooltip, when the job cannot be recrawled, and while its recrawl runs.
export function JobRecrawlButton({ job, recrawl }: { job: Job; recrawl: RecrawlControl }) {
  const check = recrawlCheck(job)
  const running = recrawl.running
  return (
    <button
      type="button"
      disabled={!check.ok || running}
      title={running ? 'Recrawling this job…' : recrawlTitle(job)}
      onClick={() => {
        void recrawl.start([job.id])
      }}
      className="flex shrink-0 items-center gap-1.5 rounded-md border border-gray-200 bg-white/70 px-3 py-1.5 text-sm text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50"
    >
      <RefreshIcon />
      {running ? 'Recrawling…' : 'Recrawl'}
    </button>
  )
}

// What came of the recrawl of this job, under the banner: nothing while there is nothing to say, a refused start or a lost batch in
// red, and once the job's crawl is over its outcome in plain words (refreshed, unchanged, filled in, kept as typed, or why it failed).
export function JobRecrawlNotice({ recrawl }: { recrawl: RecrawlControl }) {
  const { status, error } = recrawl
  let text = ''
  let ok = true
  if (error !== '') {
    text = error
    ok = false
  } else if (status !== null && !isRunning(status)) {
    const job = status.jobs.at(0)
    if (job) {
      const notice = pageNotice(job)
      text = notice.text
      ok = notice.ok
    } else if (status.cancelled > 0) {
      text = 'The recrawl was cancelled.'
      ok = false
    }
  }
  if (text === '') return null
  return (
    <div className="relative mt-3 flex items-start justify-between gap-3 text-sm">
      <p role={ok ? 'status' : 'alert'} className={ok ? 'text-green-700' : 'text-red-700'}>
        {text}
      </p>
      <button type="button" onClick={recrawl.dismiss} className="shrink-0 text-xs text-gray-500 underline hover:text-gray-700">
        Close
      </button>
    </div>
  )
}
