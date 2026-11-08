// Pure rules for the Experience page's career timeline. Months are counted as
// year * 12 + (month - 1), so a role is a half-open run of months [start, end + 1).
// No React, so they can be script-checked.
import { dateKey } from './experienceStats'

export interface TimelineEntry {
  id: string
  startDate: string
  endDate: string
}

export interface Segment {
  id: string
  // The first month and the month after the last one of the role, on the month scale.
  start: number
  end: number
  // No end date: the role runs to today.
  open: boolean
  // Which row it is drawn in, so overlapping roles do not sit on top of each other.
  lane: number
  // Position on the axis, in percent.
  left: number
  width: number
}

export interface Timeline {
  segments: Segment[]
  lanes: number
  axisStart: number
  axisEnd: number
  // Roles whose dates cannot be read (or whose end is before the start).
  undated: string[]
  // Months covered by at least one role (overlaps and gaps counted once / not at all).
  coveredMonths: number
}

// The month number of a date text, or null when it is not a readable date.
export function monthOf(text: string): number | null {
  const key = dateKey(text)
  if (key === null) return null
  return Math.floor(key / 10000) * 12 + (Math.floor(key / 100) % 100) - 1
}

export function monthOfDate(date: Date): number {
  return date.getFullYear() * 12 + date.getMonth()
}

export function buildTimeline(entries: TimelineEntry[], today: Date): Timeline {
  const todayMonth = monthOfDate(today)
  const dated: { id: string; start: number; end: number; open: boolean }[] = []
  const undated: string[] = []
  for (const e of entries) {
    const start = monthOf(e.startDate)
    const open = e.endDate.trim() === ''
    const last = open ? todayMonth : monthOf(e.endDate)
    if (start === null || last === null || last < start) {
      undated.push(e.id)
      continue
    }
    dated.push({ id: e.id, start, end: last + 1, open })
  }
  if (dated.length === 0) return { segments: [], lanes: 0, axisStart: 0, axisEnd: 0, undated, coveredMonths: 0 }

  const axisStart = Math.min(...dated.map((d) => d.start))
  const axisEnd = Math.max(...dated.map((d) => d.end))
  const span = axisEnd - axisStart

  // Earliest first; each goes in the first lane that is free by its start.
  const laneEnds: number[] = []
  const segments: Segment[] = [...dated]
    .sort((a, b) => a.start - b.start || a.end - b.end)
    .map((d) => {
      let lane = laneEnds.findIndex((end) => end <= d.start)
      if (lane === -1) lane = laneEnds.length
      laneEnds[lane] = d.end
      return { ...d, lane, left: ((d.start - axisStart) / span) * 100, width: Math.max(((d.end - d.start) / span) * 100, 1) }
    })

  let covered = 0
  let runEnd = -Infinity
  let runStart = 0
  for (const d of [...dated].sort((a, b) => a.start - b.start)) {
    if (d.start > runEnd) {
      covered += runEnd === -Infinity ? 0 : runEnd - runStart
      runStart = d.start
      runEnd = d.end
    } else if (d.end > runEnd) {
      runEnd = d.end
    }
  }
  covered += runEnd - runStart

  return { segments, lanes: laneEnds.length, axisStart, axisEnd, undated, coveredMonths: covered }
}

// "5 yrs 3 mos", "1 yr", "7 mos", "0 mos".
export function formatCovered(months: number): string {
  const years = Math.floor(months / 12)
  const rest = months % 12
  const parts: string[] = []
  if (years > 0) parts.push(`${years} ${years === 1 ? 'yr' : 'yrs'}`)
  if (rest > 0 || years === 0) parts.push(`${rest} ${rest === 1 ? 'mo' : 'mos'}`)
  return parts.join(' ')
}

export interface Tick {
  label: string
  left: number
}

// Year marks along the axis: every year, or every 2, 5 or 10 so that there are at most 8.
export function yearTicks(axisStart: number, axisEnd: number): Tick[] {
  const span = axisEnd - axisStart
  if (span <= 0) return []
  const firstYear = Math.ceil(axisStart / 12)
  const lastYear = Math.floor((axisEnd - 1) / 12)
  const count = lastYear - firstYear + 1
  const step = [1, 2, 5, 10, 20].find((s) => Math.ceil(count / s) <= 8) ?? 20
  const ticks: Tick[] = []
  for (let y = firstYear; y <= lastYear; y++) {
    if (y % step !== 0) continue
    ticks.push({ label: String(y), left: ((y * 12 - axisStart) / span) * 100 })
  }
  return ticks
}
