// Pure rules for the Jobs page's statistics strip: validating the server's answer,
// percentages, the week-on-week trend and the chart geometry. No React, so each can
// be script-checked on its own.

export interface DayCount {
  day: string
  count: number
}

export interface PlatformCount {
  name: string
  count: number
}

export interface JobStats {
  total: number
  manual: number
  companies: number
  addedLast7Days: number
  addedPrev7Days: number
  matched: number
  avgMatchScore: number | null
  withCv: number
  perDay: DayCount[]
  byPlatform: PlatformCount[]
}

type Obj = Record<string, unknown>

function isObj(v: unknown): v is Obj {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isCount(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v) && v >= 0
}

// The server's answer as JobStats, or null when it is not the shape the strip draws
// (an older or newer backend): the strip then simply hides instead of showing nonsense.
export function parseJobStats(raw: unknown): JobStats | null {
  if (!isObj(raw)) return null
  const counts = ['total', 'manual', 'companies', 'addedLast7Days', 'addedPrev7Days', 'matched', 'withCv']
  if (!counts.every((k) => isCount(raw[k]))) return null
  const avg = raw.avgMatchScore
  if (avg !== null && !(isCount(avg) && avg <= 100)) return null
  const perDay = raw.perDay
  const byPlatform = raw.byPlatform
  if (!Array.isArray(perDay) || !perDay.every((d) => isObj(d) && typeof d.day === 'string' && isCount(d.count))) return null
  if (!Array.isArray(byPlatform) || !byPlatform.every((p) => isObj(p) && typeof p.name === 'string' && isCount(p.count))) return null
  return raw as unknown as JobStats
}

// A whole percent of part in total; 0 when there is no total.
export function percentOf(part: number, total: number): number {
  return total <= 0 ? 0 : Math.round((part / total) * 100)
}

export interface Trend {
  arrow: string
  text: string
  tone: 'good' | 'neutral'
}

// This week against the one before. More new jobs is good news; fewer is just
// information, so it is not painted red. Null when both weeks are empty.
export function weekTrend(current: number, previous: number): Trend | null {
  if (current === 0 && previous === 0) return null
  if (current > previous) return { arrow: '▲', text: `+${current - previous} vs last week`, tone: 'good' }
  if (current < previous) return { arrow: '▼', text: `−${previous - current} vs last week`, tone: 'neutral' }
  return { arrow: '–', text: 'same as last week', tone: 'neutral' }
}

// How good an average match is, for colouring it.
export function scoreTone(avg: number): 'high' | 'mid' | 'low' {
  if (avg >= 70) return 'high'
  if (avg >= 40) return 'mid'
  return 'low'
}

export interface PlatformBar {
  name: string
  count: number
  // 0..100, relative to the biggest platform, so the biggest always fills the row.
  widthPct: number
}

export function platformBars(list: PlatformCount[]): PlatformBar[] {
  const max = Math.max(1, ...list.map((p) => p.count))
  return list.map((p) => ({ name: p.name, count: p.count, widthPct: Math.max(p.count > 0 ? 4 : 0, Math.round((p.count / max) * 100)) }))
}
