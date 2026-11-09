// Pure rules for the "Chrome sessions" card (plan 224): the words and numbers it shows. No React, so they can be script-checked.
import type { ChromeProcess, ChromeProcessesResponse, WatchdogStatus } from './api'

// What a Chrome is for, in the words a person knows.
export function kindLabel(kind: string): string {
  switch (kind) {
    case 'katana':
      return 'Katana engine'
    case 'rod':
      return 'Rod engine'
    case 'shared-headless':
      return 'Shared browser'
    case 'headed':
      return 'Visible fallback browser'
    case 'window':
      return 'Challenge window'
    case 'login-window':
      return 'Login window'
    default:
      return 'Chrome'
  }
}

// "45 s", "3 min", "2 h 5 min": how long something has lasted.
export function durationText(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  if (s < 60) return `${s} s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} min`
  const h = Math.floor(m / 60)
  const rest = m % 60
  return rest === 0 ? `${h} h` : `${h} h ${rest} min`
}

export type SessionsTone = 'ok' | 'busy' | 'full'

// green while there is room, amber when every slot but one is taken or someone waits, red when all slots are taken.
export function sessionsTone(running: number, limit: number, waiting: number): SessionsTone {
  if (limit > 0 && running >= limit) return 'full'
  if (waiting > 0 || (limit > 1 && running >= limit - 1)) return 'busy'
  return 'ok'
}

export function sessionsText(running: number, limit: number): string {
  return `${running} of ${limit} crawl sessions running`
}

// One flag per slot: true for a slot in use. Never more flags than the limit, and always at least none.
export function pips(running: number, limit: number): boolean[] {
  const n = Math.max(0, Math.floor(limit))
  return Array.from({ length: n }, (_, i) => i < running)
}

export function waitingText(waiting: number): string {
  return waiting === 1 ? '1 request is waiting for a free slot' : `${waiting} requests are waiting for a free slot`
}

export const EMPTY_TEXT = 'No Chrome is running. A crawl starts one, and it is closed again shortly after the crawl ends.'

// The state of one Chrome in words, and whether it needs attention.
export function stateOf(c: ChromeProcess): { text: string; bad: boolean } {
  if (c.state === 'running') return { text: c.idleSeconds < 5 ? 'working' : `idle ${durationText(c.idleSeconds)}`, bad: false }
  return { text: 'ended, being cleaned up', bad: true }
}

export interface WatchdogItem {
  label: string
  count: number
  // an event that should not normally happen: shown in amber when above zero
  attention: boolean
}

export function watchdogItems(w: WatchdogStatus): WatchdogItem[] {
  return [
    { label: 'unused Chromes stopped', count: w.killedUnused, attention: false },
    { label: 'orphan Chromes stopped', count: w.killedOrphan, attention: true },
    { label: 'ended Chromes cleaned up', count: w.deadDropped, attention: false },
    { label: 'defunct processes', count: w.defunct, attention: true },
    { label: 'stuck sessions released', count: w.stuckSessions, attention: true },
  ]
}

// "checked 12 s ago", "not checked yet".
export function lastCheckText(w: WatchdogStatus, now: number): string {
  if (w.lastRunAt === '') return 'The watchdog has not run yet'
  const t = Date.parse(w.lastRunAt)
  if (Number.isNaN(t)) return 'The watchdog has run'
  return `The watchdog checked ${durationText((now - t) / 1000)} ago`
}

// The card's one-line summary for the page header, or '' when there is nothing to say.
export function summary(r: ChromeProcessesResponse | null): string {
  return r === null ? '' : sessionsText(r.running, r.limit)
}
