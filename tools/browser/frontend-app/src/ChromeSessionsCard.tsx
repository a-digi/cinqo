import { useEffect, useState } from 'react'
import type { ChromeProcessesResponse } from './api'
import {
  EMPTY_TEXT,
  durationText,
  kindLabel,
  lastCheckText,
  pips,
  sessionsText,
  sessionsTone,
  stateOf,
  waitingText,
  watchdogItems,
  type SessionsTone,
} from './chromeSessionRules'
import { ChromeIcon } from './icons'
import { cardClassName, cardStyle } from './Shared/cardStyle'
import { PAGE_ACCENT } from './Shared/theme'
import { useChromeProcesses } from './useChromeProcesses'

const TONE_TEXT: Record<SessionsTone, string> = { ok: 'text-green-700', busy: 'text-amber-700', full: 'text-red-700' }
const TONE_PIP: Record<SessionsTone, string> = { ok: 'bg-green-500', busy: 'bg-amber-500', full: 'bg-red-500' }

// "Chrome sessions" (plan 224): how many crawl sessions run out of the most the tool allows, which Chrome processes it started and still
// tracks (with their PIDs), and what the watchdog that hunts leftover Chromes did. It reads GET /chrome-processes every few seconds while
// the page is visible, and is left out entirely by a backend that does not have the route.
export function ChromeSessionsCard() {
  const state = useChromeProcesses()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      clearInterval(t)
    }
  }, [])

  if (state.kind === 'loading' || state.kind === 'missing') return null
  if (state.kind === 'error') {
    return (
      <section className={`${cardClassName} mb-5`} style={cardStyle} aria-label="Chrome sessions">
        <Heading />
        <p role="alert" className="mt-3 text-sm text-red-700">
          {state.message}
        </p>
      </section>
    )
  }
  return <Ready data={state.data} now={now} />
}

function Heading() {
  return (
    <div className="flex items-center gap-3">
      <span
        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl text-white"
        style={{ background: `linear-gradient(135deg, ${PAGE_ACCENT.from}, ${PAGE_ACCENT.to})` }}
      >
        <ChromeIcon />
      </span>
      <h2 className="text-base font-semibold text-gray-900">Chrome sessions</h2>
    </div>
  )
}

function Ready({ data, now }: { data: ChromeProcessesResponse; now: number }) {
  const tone = sessionsTone(data.running, data.limit, data.waiting)
  return (
    <section className={`${cardClassName} mb-5`} style={cardStyle} aria-label="Chrome sessions">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Heading />
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1" aria-hidden="true">
            {pips(data.running, data.limit).map((on, i) => (
              <span key={i} className={`h-3 w-3 rounded-full transition-colors duration-500 ${on ? TONE_PIP[tone] : 'bg-gray-200'}`} />
            ))}
          </div>
          <span className={`text-sm font-semibold ${TONE_TEXT[tone]}`}>{sessionsText(data.running, data.limit)}</span>
        </div>
      </div>

      {data.waiting > 0 && (
        <p role="status" className="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
          {waitingText(data.waiting)} (they wait up to a minute, then are told to try again).
        </p>
      )}

      {data.chromes.length === 0 ? (
        <p className="mt-4 text-sm text-gray-500">{EMPTY_TEXT}</p>
      ) : (
        <ul className="mt-4 divide-y divide-gray-100 rounded-2xl border border-gray-100 bg-white">
          {data.chromes.map((c) => {
            const st = stateOf(c)
            return (
              <li key={c.pid} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 text-sm">
                <span className="min-w-0 flex-1 font-medium text-gray-900">{kindLabel(c.kind)}</span>
                <span className="font-mono text-xs text-gray-500">PID {c.pid}</span>
                <span className="text-xs text-gray-500">running for {durationText(c.ageSeconds)}</span>
                <span className={`text-xs font-medium ${st.bad ? 'text-red-700' : 'text-gray-600'}`}>{st.text}</span>
              </li>
            )
          })}
        </ul>
      )}

      <div className="mt-4 border-t border-gray-100 pt-3">
        <p className="text-xs text-gray-500">
          {lastCheckText(data.watchdog, now)}. It stops Chromes that are no longer used and hunts leftovers.
        </p>
        <dl className="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs">
          {watchdogItems(data.watchdog).map((item) => (
            <div key={item.label} className="flex items-baseline gap-1.5">
              <dd className={`font-semibold ${item.attention && item.count > 0 ? 'text-amber-700' : 'text-gray-800'}`}>{item.count}</dd>
              <dt className="text-gray-500">{item.label}</dt>
            </div>
          ))}
        </dl>
      </div>
    </section>
  )
}
