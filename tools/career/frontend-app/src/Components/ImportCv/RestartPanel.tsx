import { useEffect, useRef, useState } from 'react'
import { fetchCvText, type CVImportRun } from '../../api'
import { NEEDS_AI_REASON } from './importAiRules'
import { ImportStartPanel } from './ImportStartPanel'
import { STORED_CV_GONE_MESSAGE, storedCvAvailable } from './runOutcome'
import { useImportStart } from './useImportStart'

// Starting an import again from the page of one import (plan/ai/tools/career/step-242-restart-cv-import-inside-the-import-page.md, step 3): the same
// CV again while the stored upload can still be read (it is kept a day), or another one; the AI is picked again (it defaults to the Career settings,
// like on the upload page). It is the upload page's own start (useImportStart, ImportStartPanel), so the analysis, the checks and the move to the new
// run's own address are the same. Mounted only when the person asked for it, so nothing is read before.
export function RestartPanel({ run, onClose }: { run: CVImportRun; onClose: () => void }) {
  const start = useImportStart({ onRunsChanged: () => undefined, targetProfileId: run.targetProfileId ?? '' })
  const [stored, setStored] = useState<'checking' | 'available' | 'gone'>('checking')
  const [sameError, setSameError] = useState('')
  const top = useRef<HTMLDivElement>(null)

  // The panel opens at the top of the page, while the button that opened it (Decline all and start again) is at the bottom of a long review: it is
  // brought into view so the person sees where to go on.
  useEffect(() => {
    top.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [])

  useEffect(() => {
    let cancelled = false
    void storedCvAvailable(() => fetchCvText(run.mediaFileId)).then((ok) => {
      if (!cancelled) setStored(ok ? 'available' : 'gone')
    })
    return () => {
      cancelled = true
    }
  }, [run.mediaFileId])

  async function startSame() {
    setSameError('')
    const problem = await start.startFromStored(run.mediaFileId, run.originalFilename)
    if (problem) setSameError(problem)
  }

  return (
    <div ref={top} className="mb-5 space-y-3" aria-label="Start this import again">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-gray-900">Start this import again</h2>
        {start.state === 'upload' && (
          <button
            type="button"
            onClick={onClose}
            className="rounded-full border border-blue-200 bg-white px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
          >
            Cancel
          </button>
        )}
      </div>
      {start.state === 'upload' && stored === 'checking' && <p className="text-sm text-gray-400">Checking the uploaded CV…</p>}
      {start.state === 'upload' && stored === 'available' && (
        <div className="flex flex-wrap items-center gap-3 text-sm text-gray-600">
          <button
            type="button"
            onClick={() => {
              void startSame()
            }}
            disabled={!start.platformsLoaded || start.aiState === 'needs-ai'}
            title={start.aiState === 'needs-ai' ? NEEDS_AI_REASON : undefined}
            className="rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
          >
            Use the same CV again
          </button>
          <span>“{run.originalFilename}”, or choose another CV below.</span>
          {sameError !== '' && <span className="text-red-700">{sameError}</span>}
        </div>
      )}
      {start.state === 'upload' && stored === 'gone' && <p className="text-sm text-gray-600">{STORED_CV_GONE_MESSAGE}</p>}
      <ImportStartPanel start={start} />
    </div>
  )
}
