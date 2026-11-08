import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { TerminalViewer } from '../../Shared/Components/Terminal/TerminalViewer'
import { toolSourceId } from './fileReaderRules'
import { fileName, parseReaderQuery, type ReaderPageView } from './readerPageRules'

// The log reader as a page of its own for installed tools (plan 201): a tool sends the person here with the files it wants shown (one or
// several, as paths inside the file source it declared in its manifest) and, optionally, a way back. Everything is in the address, so a
// reload or a link in a new tab shows the same thing. The page never sends a path of its own: the viewer asks the server for the source and
// the file, and the server decides who may read what. Signed-in people only (the route has no static scope, like the reader's API).
export function ToolLogReaderPage() {
  const location = useLocation()
  const parsed = parseReaderQuery(location.search)

  if (!parsed.ok) {
    return (
      <div className="max-w-4xl space-y-4 p-6 font-sans text-gray-900">
        <BackLink back={parsed.back} label={parsed.backLabel} />
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          This link to the log reader is not valid: {parsed.message}.
        </p>
      </div>
    )
  }
  // Keyed by the address: a different address starts over with its own file selection.
  return <ReaderBody key={location.search} view={parsed.view} />
}

function BackLink({ back, label }: { back: string | null; label: string }) {
  if (back === null) return null
  return (
    <Link to={back} className="inline-flex items-center gap-1 text-sm text-blue-600 hover:underline">
      <span aria-hidden>←</span> {label}
    </Link>
  )
}

function ReaderBody({ view }: { view: ReaderPageView }) {
  // Which of the files is open; starts at the one the address names.
  const [selected, setSelected] = useState(view.open)
  const current = view.refs.includes(selected) ? selected : view.open
  const several = view.refs.length > 1
  const heading = view.title || 'Log reader'

  return (
    <div className="max-w-6xl space-y-4 p-6 font-sans text-gray-900">
      <div className="space-y-1">
        <BackLink back={view.back} label={view.backLabel} />
        <h1 className="text-xl font-semibold">{heading}</h1>
        <p className="text-sm text-gray-500">
          {several ? `${view.refs.length} files. Pick one on the left.` : 'Read a window of up to 500 lines at a time; search with /.'}
        </p>
      </div>
      <div className={several ? 'grid gap-4 lg:grid-cols-[16rem_minmax(0,1fr)]' : ''}>
        {several && (
          <ul aria-label="Files" className="max-h-48 space-y-1 overflow-auto lg:max-h-none">
            {view.refs.map((ref) => {
              const active = ref === current
              return (
                <li key={ref}>
                  <button
                    type="button"
                    aria-current={active}
                    title={ref}
                    onClick={() => {
                      setSelected(ref)
                    }}
                    className={`w-full rounded-lg border px-3 py-2 text-left font-mono text-xs transition focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                      active ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-200 bg-white text-gray-800 hover:bg-gray-50'
                    }`}
                  >
                    <span className="block truncate">{fileName(ref)}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
        <div className="min-w-0">
          <TerminalViewer
            key={`${view.tool}/${view.source}/${current}`}
            source={toolSourceId(view.tool, view.source)}
            fileRef={current}
            title={fileName(current)}
            logType={view.format}
            paneHeight="h-[calc(100vh-17rem)] min-h-[16rem]"
          />
        </div>
      </div>
    </div>
  )
}
