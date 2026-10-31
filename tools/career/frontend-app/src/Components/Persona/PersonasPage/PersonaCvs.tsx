import { useEffect, useRef, useState } from 'react'
import type { CvDocument, CvTemplate } from '../../../api'
import type { AiChoice } from '../../../Cinqo/Platform/aiChoice'
import { CreateCvWithAi } from '../../../CvAi/CreateCvWithAi'
import { failedRunToShow, hasRunningCvRun } from '../../../CvAi/cvAiRules'
import { CvAiChatModal } from '../../../CvAi/CvAiChatModal'
import { CvAiHistory } from '../../../CvAi/CvAiHistory'
import { useDismissedCvRuns } from '../../../CvAi/useDismissedCvRuns'
import { useCvAiRuns } from '../../../CvAi/useCvAiRuns'
import { mediaDownloadUrl } from '../../../api'
import { CV_BUILDER_PATH, CV_DOCUMENTS_PATH } from '../../../paths'
import { createdLabel, templateLabel } from '../../CvBuilder/cvCardRules'
import { cvsOfPersona } from '../../CvBuilder/cvPersonaRules'
import { setPersonaIdInLocalStorage } from '../PersonaSwitcher/repositoryLocalStorage'

// The CVs linked to one persona (step 114), under its card on the Personas page: the ones built
// from it and the ones attached to it. A row opens the PDF or the editor; the footer opens the CV
// Documents page on this persona, where CVs are attached and detached.
export function PersonaCvs({
  personaId,
  personaName,
  docs,
  templates,
  ai,
  onCvsChanged,
}: {
  personaId: string
  personaName: string
  docs: CvDocument[]
  templates: CvTemplate[]
  ai: AiChoice
  // Called when an AI run for this persona ended, so the page reads the CVs again.
  onCvsChanged: () => void
}) {
  const aiRuns = useCvAiRuns(personaId)
  const working = hasRunningCvRun(aiRuns.runs)
  const [dismissedRuns, dismissRun] = useDismissedCvRuns()
  const failed = failedRunToShow(aiRuns.runs, dismissedRuns)
  const workingRun = aiRuns.runs.find((r) => r.status === 'generating') ?? null
  const [chatRunId, setChatRunId] = useState<string | null>(null)
  const chatRun = aiRuns.runs.find((r) => r.id === chatRunId) ?? null
  // A run this section saw working that has ended: the CV list is read again.
  const sawWorking = useRef(false)
  useEffect(() => {
    if (working) sawWorking.current = true
    else if (sawWorking.current) {
      sawWorking.current = false
      onCvsChanged()
    }
  }, [working, onCvsChanged])
  const rows = cvsOfPersona(docs, personaId)
  const open = (path: string) => {
    window.__cinqoToolBridge.navigate(path)
  }
  return (
    <section aria-label="CVs of this persona" className="mt-3 border-t border-gray-100 pt-2">
      <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">
        {rows.length === 1 ? '1 CV' : `${rows.length} CVs`}
      </p>
      {rows.length > 0 && (
        <ul className="space-y-1">
          {rows.map(({ doc, builtFrom }) => {
            const created = createdLabel(doc.createdAt)
            return (
              <li key={doc.id} className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-600">
                <span className="min-w-0 max-w-full truncate font-medium text-gray-900" title={doc.title}>
                  {doc.title}
                </span>
                <span
                  title={builtFrom ? 'Built from this persona' : 'Attached to this persona'}
                  className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${
                    builtFrom ? 'bg-purple-100 text-purple-800' : 'bg-gray-100 text-gray-700'
                  }`}
                >
                  {builtFrom ? 'Built from this persona' : 'Attached'}
                </span>
                <span className="text-gray-400">{[templateLabel(templates, doc.templateId), created].filter((x) => x).join(' · ')}</span>
                <span className="ml-auto flex gap-2">
                  {doc.mediaFileId !== '' && (
                    <a
                      href={mediaDownloadUrl(doc.mediaFileId)}
                      target="_blank"
                      rel="noreferrer"
                      className="text-gray-600 underline hover:text-gray-900"
                    >
                      PDF
                    </a>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      open(`${CV_BUILDER_PATH}?id=${encodeURIComponent(doc.id)}`)
                    }}
                    className="text-blue-700 hover:underline"
                  >
                    Edit
                  </button>
                </span>
              </li>
            )
          })}
        </ul>
      )}
      {working && (
        <p role="status" className="mt-2 flex items-center gap-2 rounded-lg bg-purple-50 px-3 py-2 text-xs text-purple-900">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-purple-600" aria-hidden="true" />
          The AI is creating a CV. It appears here when it is done.
          {workingRun && (
            <button
              type="button"
              onClick={() => {
                setChatRunId(workingRun.id)
              }}
              className="ml-1 text-blue-700 underline"
            >
              Show the chat
            </button>
          )}
        </p>
      )}
      {failed && (
        <p role="alert" className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-800">
          The AI could not create a CV: {failed.error ?? 'no reason was recorded.'}{' '}
          <button
            type="button"
            onClick={() => {
              setChatRunId(failed.id)
            }}
            className="underline"
          >
            Show chat
          </button>{' '}
          <button
            type="button"
            onClick={() => {
              dismissRun(failed.id)
            }}
            className="underline"
          >
            Dismiss
          </button>
        </p>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <CreateCvWithAi
          ai={ai}
          personaName={personaName}
          disabledReason={
            !ai.platform
              ? 'No AI platform is available. Choose one in Settings.'
              : working
                ? 'The AI is already creating a CV for this persona.'
                : ''
          }
          onStart={(focus) => aiRuns.start({ id: personaId, name: personaName }, focus, ai)}
        />
        <button
          type="button"
          onClick={() => {
            setPersonaIdInLocalStorage(personaId)
            open(rows.length > 0 ? CV_DOCUMENTS_PATH : `${CV_BUILDER_PATH}?personaId=${encodeURIComponent(personaId)}`)
          }}
          className="text-xs text-blue-700 hover:underline"
        >
          {rows.length > 0 ? 'Manage CVs' : 'Create a CV by hand'}
        </button>
      </div>
      <CvAiHistory
        runs={aiRuns.runs}
        docs={docs}
        onShowChat={(run) => {
          setChatRunId(run.id)
        }}
      />
      <CvAiChatModal
        run={chatRun}
        onClose={() => {
          setChatRunId(null)
        }}
      />
    </section>
  )
}
