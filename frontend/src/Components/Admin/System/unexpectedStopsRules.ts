import type { SystemEventListItem } from '../../../api/system'

// unexpectedStopsRules.ts — the pure rules of the unexpected stops dashboard (plan 259): what the address says, the words of a row, and the times.

// The page sizes the table offers (the API allows 100 as well). A size in the address that is not one of these is the default.
export const STOP_PAGE_SIZES = [10, 20, 50] as const
export const DEFAULT_STOP_PAGE_SIZE = 20

// The place in the table as the address says it (?page=&pageSize=). Nothing is ever refused here: a page that is not a whole number from 1 up is the first
// page, a size that is not offered is the default (the page past the last one is brought back to the last when the answer says how many there are).
export function readListQuery(params: URLSearchParams): { page: number; pageSize: number } {
  const rawPage = params.get('page') ?? ''
  const page = /^[1-9]\d{0,6}$/.test(rawPage) ? Number(rawPage) : 1
  const size = Number(params.get('pageSize') ?? '')
  const pageSize = (STOP_PAGE_SIZES as readonly number[]).includes(size) ? size : DEFAULT_STOP_PAGE_SIZE
  return { page, pageSize }
}

// The address of a place: what is the default is left out, so the first page of the default size is the plain address.
export function listQueryString(page: number, pageSize: number): string {
  const p = new URLSearchParams()
  if (page > 1) p.set('page', String(page))
  if (pageSize !== DEFAULT_STOP_PAGE_SIZE) p.set('pageSize', String(pageSize))
  return p.toString()
}

// "12 Nov 2026, 08:30:12" in the person's own time; a time that cannot be read is shown as it is.
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const date = d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
  const time = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return `${date}, ${time}`
}

export function statusLabel(e: Pick<SystemEventListItem, 'acknowledged'>): 'New' | 'Seen' {
  return e.acknowledged ? 'Seen' : 'New'
}

// The tooltip of "Seen": when it was seen; '' for a stop that is new.
export function seenTitle(e: Pick<SystemEventListItem, 'acknowledged' | 'acknowledgedAt'>): string {
  return e.acknowledged && e.acknowledgedAt ? `Seen ${formatDateTime(e.acknowledgedAt)}` : ''
}

// The line under the title: how many stops there are and how many are new.
export function countsLine(total: number, unseen: number | null): string {
  if (total === 0) return 'Nothing has stopped unexpectedly.'
  const noun = total === 1 ? 'stop' : 'stops'
  return `${String(total)} ${noun}${unseen !== null && unseen > 0 ? `, ${String(unseen)} new` : ''}`
}

// How long ago: "just now", "12 minutes ago", "3 hours ago", "yesterday", "5 days ago"; after two weeks the date. A time in the future (a clock that is off) is "just now".
export function relativeTime(iso: string, now: Date = new Date()): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const seconds = Math.floor((now.getTime() - d.getTime()) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${String(minutes)} ${minutes === 1 ? 'minute' : 'minutes'} ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${String(hours)} ${hours === 1 ? 'hour' : 'hours'} ago`
  const days = Math.floor(hours / 24)
  if (days === 1) return 'yesterday'
  if (days < 15) return `${String(days)} days ago`
  return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

// How a kind of stop looks: its colour and its icon. A kind this page does not know is amber with the warning.
export function kindLook(kind: string): { accent: 'rose' | 'violet' | 'amber'; icon: 'alert' | 'chip' } {
  if (kind === 'local_ai_stopped') return { accent: 'violet', icon: 'chip' }
  if (kind === 'app_unclean_exit') return { accent: 'rose', icon: 'alert' }
  return { accent: 'amber', icon: 'alert' }
}

// The tile "latest new stop": how long ago the newest stop that is not seen was, or that there is none.
export function latestNewText(latest: { at: string } | undefined, now: Date = new Date()): string {
  if (latest === undefined) return 'Everything is seen'
  return relativeTime(latest.at, now) || formatDateTime(latest.at)
}
