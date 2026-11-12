import { useRef, useState } from 'react'
import { CV_IMPORT_TASK, isUnsuitableFor, modelLabel } from '../../Cinqo/Platform/modelLabels'
import { PDFIcon, UploadIcon } from '../../Shared/Icons/icons'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { FailureNotice } from './FailureNotice'
import { ImportAiPicker } from './ImportAiPicker'
import { ImportNeedsAi } from './ImportNeedsAi'
import { ImportPipeline } from './ImportPipeline'
import { UnsuitableModelNotice } from './UnsuitableModelNotice'
import { pipelineStages } from './pipelineRules'
import type { ImportStart } from './useImportStart'

// The 5 areas a CV import populates — Profile/Personas/Personal
// Details/Skills/Experience, matching the real Career entities these
// map onto. See
// plan/ai/tools/career/import-cv/step-04-import-cv-page-and-menu-entry.md.
const AREAS = ['Profile', 'Target roles', 'Personal Details', 'Skills', 'Experience'] as const

// The part of the Import page that starts an import and shows it until the run has its own page: the pipeline, the AI picker, the dropzone and its
// upload button, the analysis under way and a failure (plan/ai/tools/career/step-242-restart-cv-import-inside-the-import-page.md, step 1: moved out
// of ImportCvPage unchanged). It is fed by useImportStart.
export function ImportStartPanel({ start }: { start: ImportStart }) {
  const {
    state,
    file,
    fileName,
    uploading,
    error,
    uploadResult,
    conversationId,
    rawReply,
    chosen,
    settingsAi,
    platforms,
    platformsLoaded,
    aiState,
  } = start
  // Dropzone state — visual only (the click-to-browse path already
  // worked before; this adds a real drag-and-drop path to match the
  // dropzone's own dashed-border affordance, since offering that look
  // without the behavior would be a little deceptive).
  const [isDragOver, setIsDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const stages = pipelineStages({
    state,
    uploadError: state === 'upload' && error !== '',
    uploading,
    inserting: false,
    insertedCount: 0,
    insertErrorCount: 0,
  })

  return (
    <>
      <ImportPipeline stages={stages} onStartOver={start.startOver} />

      {state === 'upload' && aiState === 'needs-ai' && (
        <ImportNeedsAi
          onRecheck={() => {
            start.recheckAi()
          }}
        />
      )}

      {state === 'upload' && aiState !== 'needs-ai' && platformsLoaded && platforms.length > 0 && (
        <ImportAiPicker
          choice={chosen}
          settings={settingsAi}
          platforms={platforms}
          onChange={(platformId, model) => {
            start.changeAi(platformId, model)
          }}
          onReset={() => {
            start.resetAi()
          }}
        />
      )}

      {state === 'upload' &&
        aiState !== 'needs-ai' &&
        isUnsuitableFor(chosen.platform, chosen.model, CV_IMPORT_TASK) &&
        chosen.platform &&
        chosen.model && <UnsuitableModelNotice choice={`${chosen.platform.name} · ${modelLabel(chosen.platform, chosen.model)}`} />}

      {state === 'upload' && aiState !== 'needs-ai' && (
        <section className={`career-enter career-delay-1 ${cardClassName}`} style={cardStyle}>
          <div
            role="button"
            tabIndex={0}
            onClick={() => {
              fileInputRef.current?.click()
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') fileInputRef.current?.click()
            }}
            onDragOver={(e) => {
              e.preventDefault()
              setIsDragOver(true)
            }}
            onDragLeave={() => {
              setIsDragOver(false)
            }}
            onDrop={(e) => {
              e.preventDefault()
              setIsDragOver(false)
              start.pickFile(e.dataTransfer.files.length > 0 ? e.dataTransfer.files[0] : null)
            }}
            className={`flex cursor-pointer flex-col items-center rounded-2xl border-2 border-dashed px-6 py-10 text-center transition-colors ${
              isDragOver ? 'border-blue-400 bg-blue-50' : 'border-blue-200 bg-white hover:border-blue-400 hover:bg-blue-50'
            }`}
          >
            <div className="relative mb-3 h-16 w-16 text-gray-300">
              <PDFIcon className="h-16 w-16" />
              <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full bg-gray-900 text-white">
                <UploadIcon className="h-3.5 w-3.5" />
              </span>
            </div>
            <p className="mb-1 text-sm font-medium text-gray-700">{file ? file.name : 'Click to choose a PDF, or drag one here'}</p>
            <p className="text-xs text-gray-400">PDF up to 10MB</p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".pdf,application/pdf"
              onChange={(e) => {
                start.pickFile(e.target.files?.[0] ?? null)
              }}
              className="hidden"
            />
          </div>

          <div className="mt-3 min-h-[1.2em] text-sm text-red-700">{error}</div>
          <button
            type="button"
            onClick={() => {
              void start.upload()
            }}
            disabled={!file || uploading || !platformsLoaded}
            className="mt-1 rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {uploading ? 'Uploading…' : 'Upload'}
          </button>
        </section>
      )}

      {state === 'analyzing' && (
        <div className="career-stagger space-y-4">
          <p className="text-sm text-gray-500">
            Analyzing “{fileName}”…
            {uploadResult && <span className="ml-1 font-mono text-xs text-gray-400">({uploadResult.fileId})</span>}
          </p>
          {conversationId && (
            <p className="text-xs text-gray-400">
              AI conversation:{' '}
              <button type="button" onClick={start.viewTranscript} className="underline hover:text-gray-600">
                view transcript
              </button>
            </p>
          )}
          {AREAS.map((area) => (
            <section key={area} className="rounded-md border border-gray-200 p-4 shadow-sm">
              <h2 className="text-base font-semibold text-gray-900">{area}</h2>
            </section>
          ))}
        </div>
      )}

      {state === 'error' && (
        <section className={cardClassName} style={cardStyle}>
          <div className="mb-3">
            <FailureNotice error={error} onViewTranscript={conversationId ? start.viewTranscript : undefined} />
          </div>
          {rawReply && (
            <details className="mb-3">
              <summary className="cursor-pointer text-xs text-gray-500">Show the AI's last message</summary>
              <pre className="mt-2 whitespace-pre-wrap break-words text-xs text-gray-600">{rawReply}</pre>
            </details>
          )}
          <button
            type="button"
            onClick={start.startOver}
            className="rounded-full border border-blue-200 bg-white px-3.5 py-1.5 text-sm text-blue-700 hover:bg-blue-50"
          >
            Start over
          </button>
        </section>
      )}
    </>
  )
}
