import type { JobBoardProtocol } from '../../api'

// What a board card says about the protocol of its application (plan/ai/tools/career/step-176-application-protocol.md, step 4).

const MS_PER_DAY = 24 * 60 * 60 * 1000

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// The calendar day of a moment as the person lives it: a day without a time is that very day, a time is the day in the browser's zone.
function localDay(at: string): string {
  if (!at.includes('T')) return at
  const d = new Date(at)
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// How many calendar days before today the moment was (0 = today); never negative.
export function daysAgo(at: string, now: Date): number {
  const [y, m, d] = localDay(at).split('-').map(Number)
  const [ny, nm, nd] = [now.getFullYear(), now.getMonth() + 1, now.getDate()]
  const days = Math.round((Date.UTC(ny, nm - 1, nd) - Date.UTC(y, m - 1, d)) / MS_PER_DAY)
  return Math.max(0, days)
}

// "today", "yesterday", "5 days ago"; from 30 days on the date itself ("2026-09-01"), as a count of days would say little.
export function agoText(at: string, now: Date): string {
  const days = daysAgo(at, now)
  if (days === 0) return 'today'
  if (days === 1) return 'yesterday'
  if (days < 30) return `${String(days)} days ago`
  return localDay(at)
}

const WHO = { sent: 'You wrote', received: 'They wrote', note: 'Note' } as const

export interface ProtocolChip {
  // "3 entries"
  count: string
  // "You wrote 2 days ago", or '' when nothing has happened yet
  last: string
  // "1 planned", or '' when nothing is planned
  planned: string
}

export function protocolChip(p: JobBoardProtocol, now: Date): ProtocolChip {
  const last = p.lastAt === undefined ? '' : `${WHO[p.lastDirection ?? 'note']} ${agoText(p.lastAt, now)}`
  return {
    count: `${String(p.count)} ${p.count === 1 ? 'entry' : 'entries'}`,
    last,
    planned: p.planned > 0 ? `${String(p.planned)} planned` : '',
  }
}
