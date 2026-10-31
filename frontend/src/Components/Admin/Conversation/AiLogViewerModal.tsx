import { useEffect, useState } from 'react'
import { fetchTraceFiles, type AllAiTraceLogsEntry, type TraceFileEntry } from '../../../api/conversations'
import { ApiError } from '../../../api/client'
import { FullPageModal } from '../../../Shared/Components/FullPageModal/FullPageModal'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { TerminalViewer } from '../../../Shared/Components/Terminal/TerminalViewer'
import { formatBytes, traceLogHighlight } from '../../../Shared/Components/Terminal/terminalRules'
import { hasTitle, relativeTime, shortId } from './aiLogsRules'

// A conversation's AI trace files in the file reader (plan/ai/file-reader/file-reader-engine.md, step 5): the
// newest file opens straight away, the others are one click away on the left. Admin-only, like the page.
export function AiLogViewerModal({ entry, onClose }: { entry: AllAiTraceLogsEntry; onClose: () => void }) {
  const [files, setFiles] = useState<TraceFileEntry[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const [now] = useState(() => new Date())

  useEffect(() => {
    let cancelled = false
    fetchTraceFiles(entry.conversationId)
      .then((list) => {
        if (cancelled) return
        setFiles(list)
        setSelected(list[0]?.turnRunId ?? null)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(
            err instanceof ApiError && err.status === 403 ? 'You do not have access to these logs.' : 'The logs could not be listed.',
          )
      })
    return () => {
      cancelled = true
    }
  }, [entry.conversationId])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [onClose])

  const heading = hasTitle(entry) ? (entry.title ?? '') : 'Deleted conversation'

  return (
    <FullPageModal onClose={onClose}>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-6 pr-16">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">{heading}</h2>
          <p className="font-mono text-xs text-gray-500" title={entry.conversationId}>
            {shortId(entry.conversationId)} · {entry.logCount} {entry.logCount === 1 ? 'file' : 'files'}
          </p>
        </div>

        {error ? (
          <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
            {error}
          </p>
        ) : !files ? (
          <div className="flex flex-1 items-center justify-center">
            <LoadingSpinner label="Loading logs…" />
          </div>
        ) : files.length === 0 ? (
          <p className="text-sm text-gray-500">This conversation has no trace files.</p>
        ) : (
          <div className="grid gap-4 lg:grid-cols-[16rem_minmax(0,1fr)]">
            <ul aria-label="Trace files" className="max-h-48 space-y-1 overflow-auto lg:max-h-none">
              {files.map((f, i) => {
                const active = f.turnRunId === selected
                return (
                  <li key={f.turnRunId}>
                    <button
                      type="button"
                      aria-current={active}
                      onClick={() => {
                        setSelected(f.turnRunId)
                      }}
                      className={`w-full rounded-lg border px-3 py-2 text-left transition focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                        active ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-200 bg-white text-gray-800 hover:bg-gray-50'
                      }`}
                    >
                      <span className="flex items-center justify-between gap-2">
                        <span className="font-mono text-xs">{shortId(f.turnRunId)}</span>
                        {i === 0 && (
                          <span
                            className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${active ? 'bg-white/20' : 'bg-green-100 text-green-700'}`}
                          >
                            latest
                          </span>
                        )}
                      </span>
                      <span className={`mt-0.5 block text-xs ${active ? 'text-white/70' : 'text-gray-500'}`}>
                        {relativeTime(f.modifiedAt, now)} · {formatBytes(f.sizeBytes)}
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>

            {selected && (
              <TerminalViewer
                key={selected}
                source="ai-trace"
                fileRef={`${entry.conversationId}/${selected}`}
                title={`${selected}.txt`}
                highlight={traceLogHighlight}
                paneHeight="h-[calc(100vh-22rem)] min-h-[16rem]"
              />
            )}
          </div>
        )}
      </div>
    </FullPageModal>
  )
}
