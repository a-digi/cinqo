// Pure rules for the Domains page's clearance bar. No React, so they can be script-checked.
import type { DomainEntry } from './api'

export interface DomainSummary {
  total: number
  // Sites with a kept clearance, an expired one, none.
  valid: number
  expired: number
  none: number
  // Sites with a Chrome window open for someone right now.
  waiting: number
  // The soonest expiry among still-valid clearances, or null when there is none to come.
  nextExpiry: Date | null
}

export function summarizeDomains(entries: DomainEntry[], now: Date): DomainSummary {
  const s: DomainSummary = { total: entries.length, valid: 0, expired: 0, none: 0, waiting: 0, nextExpiry: null }
  for (const e of entries) {
    if (e.clearance.state === 'valid') {
      s.valid++
      const at = e.clearance.expiresAt ? new Date(e.clearance.expiresAt) : null
      if (at && !Number.isNaN(at.getTime()) && at.getTime() > now.getTime() && (s.nextExpiry === null || at < s.nextExpiry))
        s.nextExpiry = at
    } else if (e.clearance.state === 'expired') s.expired++
    else s.none++
    if (e.request?.state === 'waiting') s.waiting++
  }
  return s
}

// "under a minute", "5 min", "3 h 20 min", "2 d 4 h": how long until something, for a
// positive number of milliseconds.
export function formatIn(ms: number): string {
  const minutes = Math.floor(ms / 60000)
  if (minutes < 1) return 'under a minute'
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ${minutes % 60} min`
  return `${Math.floor(hours / 24)} d ${hours % 24} h`
}

export interface Segment {
  key: 'valid' | 'expired' | 'none'
  label: string
  count: number
  // Share of all sites, in percent (0 when there are none).
  percent: number
}

export function clearanceSegments(s: DomainSummary): Segment[] {
  const pct = (n: number) => (s.total === 0 ? 0 : (n / s.total) * 100)
  return [
    { key: 'valid', label: 'Cleared', count: s.valid, percent: pct(s.valid) },
    { key: 'expired', label: 'Expired', count: s.expired, percent: pct(s.expired) },
    { key: 'none', label: 'Not cleared', count: s.none, percent: pct(s.none) },
  ]
}
