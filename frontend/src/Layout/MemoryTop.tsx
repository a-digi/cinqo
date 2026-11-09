import { useEffect, useId, useRef, useState } from 'react'
import type { MemoryTopApp } from '../api/system'
import { FOOTNOTE, EMPTY_TEXT, LOADING_TEXT, barPercent, pidText, shareText, sizeText } from './topRules'
import { useTweenedNumber } from './tween'
import { useMemoryTop } from './useMemoryTop'

// The memory pill as a button with a dropdown under it (plan 226): the ten applications that use the most memory, biggest first. It reads the list
// only while it is open. Escape, a click outside and a second click on the pill close it; focus goes back to the pill.
export function MemoryTop({
  label,
  title,
  className,
  children,
}: {
  label: string
  title: string
  className: string
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLSpanElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const panelId = useId()
  const titleId = useId()
  const { state, retry } = useMemoryTop(open)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        buttonRef.current?.focus()
      }
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <span ref={wrapRef} className="relative inline-flex">
      <button
        ref={buttonRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        aria-label={label}
        title={title}
        onClick={() => {
          setOpen((v) => !v)
        }}
        className={`${className} cursor-pointer hover:brightness-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500`}
      >
        {children}
      </button>
      {open && (
        <div
          id={panelId}
          role="dialog"
          aria-labelledby={titleId}
          className="absolute right-0 top-full z-50 mt-2 w-[28rem] max-w-[92vw] rounded-xl border border-slate-200 bg-white p-3 text-left shadow-xl"
        >
          <p id={titleId} className="text-sm font-semibold text-slate-900">
            Top 10 by memory
          </p>
          <Body state={state} retry={retry} />
          <p className="mt-2 border-t border-slate-100 pt-2 text-[11px] leading-snug text-slate-500">{FOOTNOTE}</p>
        </div>
      )}
    </span>
  )
}

function Body({ state, retry }: { state: ReturnType<typeof useMemoryTop>['state']; retry: () => void }) {
  if (state.kind === 'loading') return <p className="mt-2 text-xs text-slate-500">{LOADING_TEXT}</p>
  if (state.kind === 'error') {
    return (
      <div className="mt-2 text-xs">
        <p role="alert" className="text-red-700">
          {state.message}
        </p>
        <button
          type="button"
          onClick={retry}
          className="mt-1 rounded-md px-2 py-1 font-medium text-slate-700 ring-1 ring-slate-300 hover:bg-slate-50"
        >
          Retry
        </button>
      </div>
    )
  }
  const { apps, totalBytes, note } = state.data
  if (apps.length === 0) return <p className="mt-2 text-xs text-slate-500">{note ?? EMPTY_TEXT}</p>
  const biggest = apps.reduce((m, a) => Math.max(m, a.rssBytes), 0)
  return (
    <ol className="mt-2 space-y-1.5">
      {apps.map((a, i) => (
        <Row key={a.cinqo ? '\u0000cinqo' : a.name} app={a} rank={i + 1} totalBytes={totalBytes} biggest={biggest} />
      ))}
    </ol>
  )
}

// One application. Its size glides when it changes; the bar grows and shrinks with a width transition.
function Row({ app, rank, totalBytes, biggest }: { app: MemoryTopApp; rank: number; totalBytes: number; biggest: number }) {
  const shown = useTweenedNumber(app.rssBytes)
  return (
    <li className="grid grid-cols-[1.25rem_1fr_auto] items-center gap-x-2 text-xs">
      <span className="text-right text-slate-400">{rank}</span>
      <div className="min-w-0">
        <div className="flex items-baseline gap-1.5">
          <span className="truncate font-medium text-slate-900">{app.name}</span>
          {app.cinqo && (
            <span className="shrink-0 rounded-full bg-teal-50 px-1.5 py-0.5 text-[10px] font-semibold text-teal-700 ring-1 ring-teal-200">
              yours
            </span>
          )}
        </div>
        <div className="text-[11px] text-slate-500">{pidText(app.pid, app.processes)}</div>
        <div aria-hidden="true" className="mt-1 h-1 rounded-full bg-slate-100">
          <div
            data-testid="share-bar"
            className="h-full rounded-full bg-slate-400 transition-[width] duration-700 ease-out"
            style={{ width: `${barPercent(app.rssBytes, biggest)}%` }}
          />
        </div>
      </div>
      <div className="text-right">
        <div className="font-semibold text-slate-900">{sizeText(shown)}</div>
        <div className="text-[11px] text-slate-500">{shareText(app.rssBytes, totalBytes)}</div>
      </div>
    </li>
  )
}
