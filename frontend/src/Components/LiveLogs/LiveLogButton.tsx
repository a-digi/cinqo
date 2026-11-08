import { useEffect, useMemo, useState } from 'react'
import type { Highlighter } from './highlight'
import { liveFeedFor } from './liveLogsApi'
import { LiveLogViewer } from './LiveLogViewer'
import { useLiveProviders } from './useLiveProviders'

export interface LiveLogButtonProps {
  // Which kind of log (a provider id such as "ai-trace") and which one (the reference the server resolves, never a path).
  provider: string
  fileRef: string
  // What the title bar calls it.
  title: string
  // Open the live view at once when it is shown (a log that is still being written).
  startOpen?: boolean
  highlight?: Highlighter
  paneHeight?: string
  // Tells the page whether the live view is open, so it can put its own view of the same log aside.
  onOpenChange?: (open: boolean) => void
}

// The one thing a page puts next to a log to offer a live view: a "Live" button that opens a LiveLogViewer below it. It renders nothing when
// the server does not offer a live view of this kind of log to this person (not an administrator, or the live log module is not there), so the
// page needs no condition of its own and removing the module leaves nothing behind.
export function LiveLogButton({ provider, fileRef, title, startOpen = false, highlight, paneHeight, onOpenChange }: LiveLogButtonProps) {
  const providers = useLiveProviders()
  const available = providers?.some((p) => p.id === provider) ?? false
  const [chosen, setChosen] = useState<boolean | null>(null)
  const open = available && (chosen ?? startOpen)
  const feed = useMemo(() => liveFeedFor(provider, fileRef), [provider, fileRef])

  useEffect(() => {
    onOpenChange?.(open)
  }, [open, onOpenChange])

  if (!available) return null
  return (
    <div className="flex flex-col gap-3">
      <div>
        <button
          type="button"
          aria-expanded={open}
          onClick={() => {
            setChosen(!open)
          }}
          className={`inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 ${
            open ? 'border-red-300 bg-red-50 text-red-700' : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-50'
          }`}
        >
          <span aria-hidden="true" className={`h-2 w-2 rounded-full ${open ? 'bg-red-500' : 'bg-gray-400'}`} />
          {open ? 'Close live view' : 'Live'}
        </button>
      </div>
      {open && <LiveLogViewer feed={feed} name={title} highlight={highlight} paneHeight={paneHeight} />}
    </div>
  )
}
