import { useEffect, useState } from 'react'
import { fetchCVImportRuns, deleteCVImportRun, type CVImportRun } from '../../api'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { UploadIcon } from '../../Shared/Icons/icons'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { plural } from '../CompaniesPage/companyStats'
import { runBadge } from './runStatusRules'
import { IMPORT_CV_PATH, importCvRunPathFor } from '../../paths'
import { FailureNotice } from './FailureNotice'
import { ImportStartPanel } from './ImportStartPanel'
import { useImportStart } from './useImportStart'
import { PageTips } from '../../Shared/PageTips/PageTips'

// Import CV's own page — upload a CV PDF (or process an earlier one again) and the import history. The analysis itself starts here:
// an AI conversation reads the file and hands a proposal over (step 5). As soon as the run is recorded the page moves to the run's
// own URL (ImportRunPage, plan 166), where the analysis is waited for and the proposal is reviewed (ImportReview) and inserted.
// The conversation is a normal, visible one (not hidden): the user gets a full, inspectable transcript.
export function ImportCvPage() {
  // Import history — every past analysis (AI proposal + eventual save
  // outcome), newest first. Deletable, viewable read-only, and
  // reprocessable (re-runs the AI against the same underlying upload,
  // skipping re-upload) — see plan/ai/media/step-05-career-history.md.
  const [runs, setRuns] = useState<CVImportRun[] | null>(null)
  const [runActionError, setRunActionError] = useState('')

  function loadRuns() {
    fetchCVImportRuns()
      .then(setRuns)
      .catch(() => {
        // Non-fatal, matches the platforms effect's own reasoning above
        // — the upload flow itself doesn't depend on this list, so a
        // failed load just leaves the section empty rather than
        // blocking anything.
        setRuns([])
      })
  }

  useEffect(() => {
    loadRuns()
  }, [])

  // Starting an import (the AI choice, the upload, the analysis) is the hook's: the import's own page uses the same one (plan 242).
  const start = useImportStart({ onRunsChanged: loadRuns })

  async function handleProcessAgain(run: CVImportRun) {
    setRunActionError('')
    const problem = await start.startFromStored(run.mediaFileId, run.originalFilename)
    if (problem) setRunActionError(problem)
  }

  async function handleDeleteRun(run: CVImportRun) {
    setRunActionError('')
    try {
      await deleteCVImportRun(run.id)
      setRuns((prev) => (prev ? prev.filter((r) => r.id !== run.id) : prev))
    } catch (err) {
      setRunActionError(err instanceof Error ? err.message : String(err))
    }
  }

  // The header's pill is the history count; where the import is lives in the pipeline.
  const headerPill = runs !== null && runs.length > 0 ? { text: plural(runs.length, 'past import', 'past imports') } : undefined

  return (
    <div className="career-v-swing career-a-glow career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UploadIcon className="h-6 w-6" />}
        title="Import from CV"
        help={IMPORT_CV_PATH}
        description="Upload a CV and let the AI propose a profile, target role, summary, skills, experience and education from it. You review everything before it is saved."
        pill={headerPill}
      />
      {/* Steps 105 to 108: the tips sit in a column of their own beside the page (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <ImportStartPanel start={start} />

          {start.state === 'upload' && runs !== null && runs.length > 0 && (
            <section className={`career-enter career-delay-2 mt-4 ${cardClassName}`} style={cardStyle}>
              <h2 className="mb-2 text-sm font-semibold text-gray-900">Import history</h2>
              <div className="mb-2 min-h-[1.2em] text-sm text-red-700">{runActionError}</div>
              <ul className="divide-y divide-gray-100">
                {runs.map((run) => {
                  const badge = runBadge(run)
                  return (
                    <li key={run.id} className="flex flex-wrap items-center justify-between gap-3 py-2 text-sm">
                      <div className="min-w-0">
                        <a
                          href={importCvRunPathFor(run.id)}
                          onClick={(e) => {
                            e.preventDefault()
                            window.__cinqoToolBridge.navigate(importCvRunPathFor(run.id))
                          }}
                          className="block truncate text-gray-700 hover:text-blue-700 hover:underline"
                        >
                          {run.originalFilename}
                        </a>
                        <p className="text-xs text-gray-400">{new Date(run.createdAt).toLocaleString()}</p>
                      </div>
                      <div className="flex shrink-0 items-center gap-3">
                        {badge && (
                          <span
                            title={badge.title}
                            className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.tone === 'red' ? 'bg-red-100 text-red-700' : badge.tone === 'gray' ? 'bg-gray-100 text-gray-600' : 'bg-amber-100 text-amber-700'}`}
                          >
                            {badge.label}
                          </span>
                        )}
                        <a
                          href={importCvRunPathFor(run.id)}
                          onClick={(e) => {
                            e.preventDefault()
                            window.__cinqoToolBridge.navigate(importCvRunPathFor(run.id))
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Open
                        </a>
                        <button
                          type="button"
                          onClick={() => {
                            void handleProcessAgain(run)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Process again
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            void handleDeleteRun(run)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </div>
                      {run.status === 'failed' && (
                        <div className="w-full">
                          <FailureNotice error={run.error} />
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            </section>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="importCv" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
