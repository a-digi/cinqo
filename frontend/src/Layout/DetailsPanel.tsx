import { useEffect, useId, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { SystemResources } from '../api/system'
import { UNEXPECTED_STOPS_PATH } from '../config/routing/systemPaths'
import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { agentLine, chipText, deviceRows, liveRows, stopsLine } from './eventsRules'
import { AlertIcon, LaptopIcon } from './ResourceIcons'
import { useFlash } from './tween'
import { useSystemEvents } from './useSystemEvents'

// The details of the computer and of Cinqo's own record of unexpected stops (plan 223, step 5), in a panel under two buttons of the bar: "Details"
// (always) and, while a stop has not been marked as seen, a red chip that says so. Both open the same panel. The panel only says how many stops there are and
// how many are new, with a link to their own page (plan 259): the table, the facts of each stop and marking one as seen are there.
export function SystemDetails({ resources, cinqoWatch }: { resources: SystemResources; cinqoWatch: string }) {
  const flash = useFlash(cinqoWatch) // Cinqo's own memory lights up when its real reading changes
  const { summary } = useSystemEvents()
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLSpanElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const panelId = useId()
  const titleId = useId()
  const chip = chipText(summary)

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

  const toggle = () => {
    setOpen((v) => !v)
  }

  return (
    <span ref={wrapRef} className="relative inline-flex items-center gap-2">
      {chip !== '' && (
        <button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-controls={open ? panelId : undefined}
          onClick={toggle}
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-full bg-red-600 px-2.5 py-1 text-xs font-semibold text-white shadow-sm ring-1 ring-red-700/30 hover:bg-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-400"
        >
          <AlertIcon className="h-3.5 w-3.5" />
          <span className="max-lg:hidden">{chip} · details</span>
          <span className="lg:hidden">Stopped</span>
        </button>
      )}
      <button
        ref={buttonRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        aria-label="Details about this computer and Cinqo"
        title={`Details about this computer and Cinqo. Cinqo uses ${formatBytes(resources.cinqo.appBytes)} of memory.`}
        onClick={toggle}
        className={`inline-flex cursor-pointer items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium text-slate-700 shadow-sm ring-1 ring-inset ring-slate-200 transition-all duration-500 hover:bg-slate-50 ${flash ? 'scale-105 bg-sky-50 shadow-md' : 'bg-white'} focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500`}
      >
        <LaptopIcon className="h-3.5 w-3.5" />
        <span className="max-md:hidden">Details</span>
        <span className="text-slate-400 max-md:hidden" aria-hidden="true">·</span>
        <span className="tabular-nums text-slate-500">Cinqo {formatBytes(resources.cinqo.appBytes)}</span>
      </button>
      {open && (
        <div
          id={panelId}
          role="dialog"
          aria-labelledby={titleId}
          className="absolute left-0 top-full z-50 mt-2 max-h-[75vh] w-[32rem] max-w-[92vw] overflow-y-auto rounded-xl border border-slate-200 bg-white p-4 text-left shadow-xl"
        >
          <h2 id={titleId} className="text-sm font-semibold text-slate-900">
            This computer and Cinqo
          </h2>
          <Section title="This device">
            <Facts rows={deviceRows(resources)} />
          </Section>
          <Section title="Right now">
            <Facts rows={liveRows(resources)} />
          </Section>
          <Section title="Local agents">
            <p className="text-xs text-slate-700">{agentLine(resources)}</p>
          </Section>
          <Section title="Unexpected stops">
            <p data-testid="stops-line" className="text-xs text-slate-700">
              {stopsLine(summary)}
            </p>
            {summary.total > 0 && (
              <Link
                to={UNEXPECTED_STOPS_PATH}
                onClick={() => {
                  setOpen(false)
                }}
                className="mt-1.5 inline-block text-xs font-semibold text-blue-700 underline-offset-2 hover:underline"
              >
                Open the unexpected stops →
              </Link>
            )}
          </Section>
        </div>
      )}
    </span>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-3 border-t border-slate-100 pt-3 first-of-type:border-t-0">
      <h3 className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">{title}</h3>
      {children}
    </section>
  )
}

function Facts({ rows }: { rows: { label: string; value: string }[] }) {
  return (
    <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-0.5 text-xs">
      {rows.map((r) => (
        <div key={r.label} className="contents">
          <dt className="text-slate-500">{r.label}</dt>
          <dd className="font-medium text-slate-900">{r.value}</dd>
        </div>
      ))}
    </dl>
  )
}
