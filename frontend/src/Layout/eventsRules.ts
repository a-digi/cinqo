import type { SystemEvent, SystemEventsSummary, SystemResources } from '../api/system'
import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { agentView, deviceText } from './resourceLevel'

// Pure rules for the details panel (plan 223, step 5): which stops are unseen, the words of the red chip, the time of a stop, and the rows of "right now".

export const EVENTS_POLL_MS = 30_000

// "14:02" for today, "9 Nov 14:02" for another day (local time, English words).
export function whenText(iso: string, now: Date = new Date()): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const time = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
  const sameDay = d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate()
  return sameDay ? time : `${d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })} ${time}`
}

// The red chip: the newest unseen stop and how many more are waiting; '' when nothing is unseen.
export function chipText(summary: SystemEventsSummary, now: Date = new Date()): string {
  if (summary.unseen === 0 || summary.latest === undefined) return ''
  const more = summary.unseen - 1
  return `Stopped unexpectedly ${whenText(summary.latest.at, now)}${more > 0 ? ` (+${String(more)} earlier)` : ''}`
}

// The one line of the dropdown's "Unexpected stops" section (plan 259): how many there are and how many are new; the stops themselves are on their own page.
export function stopsLine(summary: SystemEventsSummary): string {
  if (summary.total === 0) return 'No unexpected stops recorded.'
  const noun = summary.total === 1 ? 'unexpected stop' : 'unexpected stops'
  return `${String(summary.total)} ${noun} recorded${summary.unseen > 0 ? `, ${String(summary.unseen)} new` : ''}`
}

export function eventTitle(e: Pick<SystemEvent, 'kind'>): string {
  return e.kind === 'local_ai_stopped' ? 'The local model stopped' : 'Cinqo stopped'
}

export interface FactRow {
  label: string
  value: string
}

// The device facts, one per row, leaving out what is unknown.
export function deviceRows(r: SystemResources): FactRow[] {
  const d = r.device
  const rows: FactRow[] = [
    { label: 'Model', value: d.model !== '' ? d.model : d.modelId },
    { label: 'Chip', value: d.chip },
    { label: 'System', value: [d.os, d.osVersion].filter((x) => x !== '').join(' ') },
    { label: 'Architecture', value: d.arch !== '' ? `${d.arch}, ${String(d.cores)} cores` : '' },
    { label: 'Memory', value: r.memory.totalBytes > 0 ? formatBytes(r.memory.totalBytes) : '' },
  ]
  return rows.filter((x) => x.value !== '')
}

// The numbers of the bar, written out.
export function liveRows(r: SystemResources): FactRow[] {
  const m = r.memory
  const rows: FactRow[] = []
  if (m.totalBytes > 0) {
    rows.push({ label: 'Memory used', value: `${formatBytes(m.totalBytes - m.availableBytes)} of ${formatBytes(m.totalBytes)}` })
    rows.push({ label: 'Memory free', value: formatBytes(m.availableBytes) })
  }
  if (m.pressure !== 'unknown') rows.push({ label: 'Memory pressure', value: m.pressure })
  rows.push({ label: 'Swap in use', value: formatBytes(m.swapUsedBytes) })
  if (r.cpu.load1 >= 0 && r.cpu.cores > 0)
    rows.push({ label: 'CPU load', value: `${r.cpu.load1.toFixed(1)} of ${String(r.cpu.cores)} cores` })
  if (r.disk.totalBytes > 0)
    rows.push({ label: 'Disk free', value: `${formatBytes(r.disk.freeBytes)} of ${formatBytes(r.disk.totalBytes)}` })
  rows.push({ label: 'Cinqo', value: formatBytes(r.cinqo.appBytes) })
  return rows
}

export function agentLine(r: SystemResources): string {
  return agentView(r.agents).text
}

export { deviceText }
