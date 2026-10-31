import { useState } from 'react'
import type { CvAiRun, CvDocument } from '../api'
import { formatRunTime } from '../PersonaAi/runDisplay'
import { runBadge, visibleCvRuns } from './cvAiRules'

const TONE = {
  working: 'bg-purple-100 text-purple-800',
  ready: 'bg-green-100 text-green-800',
  failed: 'bg-red-100 text-red-800',
} as const

// The history of a persona's AI CV runs (step 115), newest first: how each went, what it was for, the
// CV it created, and its chat, which opens in a modal. Runs are never deleted from here, so every
// chat stays reachable.
export function CvAiHistory({ runs, docs, onShowChat }: { runs: CvAiRun[]; docs: CvDocument[]; onShowChat: (run: CvAiRun) => void }) {
  const [showAll, setShowAll] = useState(false)
  if (runs.length === 0) return null
  const { shown, hidden } = visibleCvRuns(runs, showAll)
  return (
    <section aria-label="AI CV history" className="mt-2 rounded-lg bg-gray-50 px-3 py-2">
      <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">AI history</p>
      <ul className="space-y-1.5">
        {shown.map((run) => {
          const badge = runBadge(run)
          const cv = docs.find((d) => d.id === run.cvDocumentId)
          return (
            <li key={run.id} className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-600">
              <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${TONE[badge.tone]}`}>{badge.label}</span>
              <span className="text-gray-400">{formatRunTime(run.createdAt)}</span>
              {run.focus !== '' && (
                <span className="min-w-0 max-w-full truncate" title={run.focus}>
                  For: {run.focus}
                </span>
              )}
              {cv && (
                <span className="min-w-0 max-w-full truncate font-medium text-gray-900" title={cv.title}>
                  {cv.title}
                </span>
              )}
              <button
                type="button"
                onClick={() => {
                  onShowChat(run)
                }}
                className="ml-auto text-blue-700 hover:underline"
              >
                Show chat
              </button>
            </li>
          )
        })}
      </ul>
      {hidden > 0 && (
        <button
          type="button"
          onClick={() => {
            setShowAll(true)
          }}
          className="mt-1.5 text-xs text-blue-700 hover:underline"
        >
          Show all {runs.length} runs
        </button>
      )}
    </section>
  )
}
