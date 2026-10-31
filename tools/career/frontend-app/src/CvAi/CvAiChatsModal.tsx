import type { CvAiRun, CvDocument } from '../api'
import { Modal } from '../Shared/Modal/Modal'
import { formatRunTime } from '../PersonaAi/runDisplay'
import { openRunChat, runBadge } from './cvAiRules'

const TONE = {
  working: 'bg-purple-100 text-purple-800',
  ready: 'bg-green-100 text-green-800',
  failed: 'bg-red-100 text-red-800',
} as const

// The list of a persona's AI CV chats (step 115), newest first, in a modal: how each run went, what it
// was for and the CV it created. The chat itself is not shown here: "Open chat" opens it in the chat
// window at the bottom. Runs are never deleted from here, so every chat stays reachable.
export function CvAiChatsModal({ open, runs, docs, onClose }: { open: boolean; runs: CvAiRun[]; docs: CvDocument[]; onClose: () => void }) {
  return (
    <Modal open={open} title="AI chats for CVs" onClose={onClose}>
      {runs.length === 0 ? (
        <p className="text-sm text-gray-500">No AI chats yet.</p>
      ) : (
        <ul className="max-h-[60vh] space-y-2 overflow-y-auto pr-1">
          {runs.map((run) => {
            const badge = runBadge(run)
            const cv = docs.find((d) => d.id === run.cvDocumentId)
            return (
              <li key={run.id} className="rounded-xl border border-gray-200 px-3 py-2.5">
                <div className="flex flex-wrap items-center gap-2 text-xs text-gray-500">
                  <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${TONE[badge.tone]}`}>{badge.label}</span>
                  <span>{formatRunTime(run.createdAt)}</span>
                  <button
                    type="button"
                    onClick={() => {
                      openRunChat(run)
                      onClose()
                    }}
                    className="ml-auto rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                  >
                    Open chat
                  </button>
                </div>
                {run.focus !== '' && <p className="mt-1.5 break-words text-sm text-gray-700">For: {run.focus}</p>}
                {cv && <p className="mt-0.5 break-words text-sm font-medium text-gray-900">{cv.title}</p>}
                {run.status === 'failed' && run.error && <p className="mt-0.5 break-words text-xs text-red-700">{run.error}</p>}
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}
