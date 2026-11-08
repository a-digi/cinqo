import { useEffect, useRef, useState } from 'react'
import type { MissingDetailState } from '../../api'
import { noDetailsNote } from './noDetailsRules'

// The small icon after the title of a job whose details were not crawled (plan 214). Clicking it shows what is going on: running, queued, scheduled, or why
// nothing is coming. The note opens in place, closes on a second click, on Escape or on a click elsewhere, and never opens the job.
export function NoDetailsIcon({ state, platformName }: { state: MissingDetailState | undefined; platformName: string | undefined }) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    if (!open) return
    function away(e: MouseEvent) {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false)
    }
    function escape(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', away)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', away)
      document.removeEventListener('keydown', escape)
    }
  }, [open])
  return (
    <span ref={box} className="relative inline-flex shrink-0">
      <button
        type="button"
        aria-label="No details yet"
        aria-expanded={open}
        data-testid="no-details-icon"
        onClick={(e) => {
          e.stopPropagation()
          setOpen((v) => !v)
        }}
        className="rounded-full text-amber-600 hover:text-amber-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500"
      >
        <svg viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Zm-7-4a1 1 0 1 1-2 0 1 1 0 0 1 2 0ZM9 9a1 1 0 0 0 0 2v3a1 1 0 0 0 1 1h1a1 1 0 1 0 0-2v-3a1 1 0 0 0-1-1H9Z"
            clipRule="evenodd"
          />
        </svg>
      </button>
      {open && (
        <span
          role="note"
          data-testid="no-details-note"
          className="absolute left-0 top-6 z-20 w-64 rounded-lg border border-gray-200 bg-white p-3 text-xs font-normal text-gray-700 shadow-lg"
        >
          {noDetailsNote(state, platformName)}
        </span>
      )}
    </span>
  )
}
