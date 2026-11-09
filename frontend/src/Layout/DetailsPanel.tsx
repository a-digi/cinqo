import { useEffect, useId, useRef, useState } from 'react'
import { acknowledgeSystemEvent, type SystemEvent, type SystemResources } from '../api/system'
import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { agentLine, chipText, deviceRows, eventTitle, liveRows, unseenOf, whenText } from './eventsRules'
import { AlertIcon, LaptopIcon } from './ResourceIcons'
import { useFlash } from './tween'
import { useSystemEvents } from './useSystemEvents'

// The details of the computer and of Cinqo's own record of unexpected stops (plan 223, step 5), in a panel under two buttons of the bar: "Details"
// (always) and, while a stop has not been marked as seen, a red chip that says so. Both open the same panel. Marking a stop as seen needs the
// manage scope; without it the panel only shows the record.
export function SystemDetails({ resources, canManage, cinqoWatch }: { resources: SystemResources; canManage: boolean; cinqoWatch: string }) {
  const flash = useFlash(cinqoWatch) // Cinqo's own memory lights up when its real reading changes
  const { events, refresh } = useSystemEvents()
  const unseen = unseenOf(events)
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLSpanElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const panelId = useId()
  const titleId = useId()
  const chip = chipText(unseen)

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
          className="absolute right-0 top-full z-50 mt-2 max-h-[75vh] w-[32rem] max-w-[92vw] overflow-y-auto rounded-xl border border-slate-200 bg-white p-4 text-left shadow-xl"
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
            <Stops events={events} canManage={canManage} onSeen={refresh} />
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

function Stops({ events, canManage, onSeen }: { events: SystemEvent[]; canManage: boolean; onSeen: () => void }) {
  if (events.length === 0) return <p className="text-xs text-slate-500">No unexpected stops recorded.</p>
  return (
    <ul className="space-y-2">
      {events.map((e) => (
        <Stop key={e.id} event={e} canManage={canManage} onSeen={onSeen} />
      ))}
    </ul>
  )
}

function Stop({ event, canManage, onSeen }: { event: SystemEvent; canManage: boolean; onSeen: () => void }) {
  const [facts, setFacts] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const seen = event.acknowledged

  const markSeen = async () => {
    setBusy(true)
    setError('')
    try {
      await acknowledgeSystemEvent(event.id)
      onSeen()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <li
      className={`rounded-lg border px-3 py-2 text-xs ${seen ? 'border-slate-200 bg-slate-50 text-slate-600' : 'border-red-200 bg-red-50 text-slate-900'}`}
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-semibold">
          {eventTitle(event)} at {whenText(event.at)}
        </span>
        {seen ? (
          <span className="shrink-0 text-[11px] text-slate-500">seen</span>
        ) : (
          <span className="shrink-0 text-[11px] font-semibold text-red-700">new</span>
        )}
      </div>
      <p className="mt-1 leading-snug">{event.verdict}</p>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        {event.evidence.length > 0 && (
          <button
            type="button"
            aria-expanded={facts}
            onClick={() => {
              setFacts((v) => !v)
            }}
            className="font-medium text-slate-700 underline underline-offset-2 hover:no-underline"
          >
            {facts ? 'Hide facts' : 'Show facts'}
          </button>
        )}
        {!seen && canManage && (
          <button
            type="button"
            disabled={busy}
            onClick={() => void markSeen()}
            className="rounded-md bg-slate-800 px-2.5 py-1 font-semibold text-white hover:bg-slate-700 disabled:opacity-60"
          >
            {busy ? 'Marking…' : 'Mark as seen'}
          </button>
        )}
      </div>
      {error !== '' && (
        <p role="alert" className="mt-1 text-red-700">
          {error}
        </p>
      )}
      {facts && (
        <dl className="mt-2 grid grid-cols-[9rem_1fr] gap-x-3 gap-y-0.5 border-t border-slate-200 pt-2">
          {event.evidence.map((f) => (
            <div key={f.label} className="contents">
              <dt className="text-slate-500">{f.label}</dt>
              <dd className="break-words font-medium">{f.value}</dd>
            </div>
          ))}
        </dl>
      )}
    </li>
  )
}
