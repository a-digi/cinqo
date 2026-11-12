// The numbers of each profile's applications on the Profiles page (plan 265): reading the answer of GET /application-count, and the words and tiles
// the page shows for them. Pure (no React), so they can be script-checked. An "application" is the one rule of the Applications page: a job in the
// Applied status on the profile's board, or with entries in its protocol; `applied` and `waiting` are two parts of it that can overlap.

export interface ApplicationNumbers {
  total: number
  // In the Applied status on the profile's board.
  applied: number
  // The other side owes a reply.
  waiting: number
}

// What is known about the numbers of one profile: still being read, not known (the request failed, the answer was not what was expected, or the profile
// is not in it: a wrong 0 is never shown), or the numbers.
export type ApplicationNumbersView = { kind: 'loading' } | { kind: 'unknown' } | { kind: 'known'; numbers: ApplicationNumbers }

// What a page keeps: nothing yet, nothing usable, or the numbers by profile id.
export type ApplicationNumbersState =
  { status: 'loading' } | { status: 'unknown' } | { status: 'known'; byProfile: Record<string, ApplicationNumbers> }

function isCount(v: unknown): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= 0
}

// The numbers by profile id from the answer ({ total, profiles: [{ profileId, total, applied, waiting }] }), or null when the answer has no usable list.
// An entry that is not exactly that (no id, a number that is not a whole number of at least 0, applied or waiting above the total, which can never be) is
// left out, so that profile shows nothing rather than something wrong; the first entry of a profile counts.
export function parseApplicationCounts(raw: unknown): Record<string, ApplicationNumbers> | null {
  if (typeof raw !== 'object' || raw === null) return null
  const list = (raw as { profiles?: unknown }).profiles
  if (!Array.isArray(list)) return null
  const out: Record<string, ApplicationNumbers> = {}
  for (const item of list as unknown[]) {
    if (typeof item !== 'object' || item === null) continue
    const e = item as { profileId?: unknown; total?: unknown; applied?: unknown; waiting?: unknown }
    if (typeof e.profileId !== 'string' || e.profileId === '' || e.profileId in out) continue
    if (!isCount(e.total) || !isCount(e.applied) || !isCount(e.waiting)) continue
    if (e.applied > e.total || e.waiting > e.total) continue
    out[e.profileId] = { total: e.total, applied: e.applied, waiting: e.waiting }
  }
  return out
}

// The state for a finished read: unknown when the answer had no usable list.
export function stateFromAnswer(raw: unknown): ApplicationNumbersState {
  const byProfile = parseApplicationCounts(raw)
  return byProfile === null ? { status: 'unknown' } : { status: 'known', byProfile }
}

export function numbersFor(state: ApplicationNumbersState, profileId: string): ApplicationNumbersView {
  if (state.status === 'loading') return { kind: 'loading' }
  if (state.status === 'unknown') return { kind: 'unknown' }
  const numbers = Object.hasOwn(state.byProfile, profileId) ? state.byProfile[profileId] : undefined
  return numbers ? { kind: 'known', numbers } : { kind: 'unknown' }
}

// "No applications yet", "1 application", "7 applications".
export function applicationsLine(total: number): string {
  if (total <= 0) return 'No applications yet'
  return total === 1 ? '1 application' : `${String(total)} applications`
}

// The text of the number pill on the card of the job board: the figure, or nothing while it is not known.
export function applicationsPill(view: ApplicationNumbersView): string | null {
  return view.kind === 'known' ? String(view.numbers.total) : null
}

// The tooltip of the card: "Applications of Ada Lovelace: 7 (3 applied, 2 waiting for a reply)"; just the name's line when the numbers are not known.
export function applicationsTitle(name: string, view: ApplicationNumbersView): string {
  if (view.kind !== 'known') return `Applications of ${name}`
  const n = view.numbers
  if (n.total === 0) return `Applications of ${name}: none yet`
  const parts = [`${String(n.applied)} applied`]
  if (n.waiting > 0) parts.push(`${String(n.waiting)} waiting for a reply`)
  return `Applications of ${name}: ${String(n.total)} (${parts.join(', ')})`
}

export type StripTileKey = 'total' | 'applied' | 'waiting'

export interface StripTile {
  key: StripTileKey
  label: string
  value: number
  // Draw this tile with attention (something waits for a reply).
  emphasis: boolean
}

// What the numbers strip shows: nothing while the numbers are not known or are being read (a placeholder is drawn while reading), the quiet "no
// applications yet" tile for zero, else the three tiles.
export type StripView =
  { kind: 'hidden' } | { kind: 'loading' } | { kind: 'empty'; text: string; hint: string } | { kind: 'numbers'; tiles: StripTile[] }

export const EMPTY_HINT = 'Move a job to Applied on the job board.'

export function stripFor(view: ApplicationNumbersView): StripView {
  if (view.kind === 'loading') return { kind: 'loading' }
  if (view.kind === 'unknown') return { kind: 'hidden' }
  const n = view.numbers
  if (n.total === 0) return { kind: 'empty', text: applicationsLine(0), hint: EMPTY_HINT }
  return {
    kind: 'numbers',
    tiles: [
      { key: 'total', label: n.total === 1 ? 'Application' : 'Applications', value: n.total, emphasis: false },
      { key: 'applied', label: 'Applied', value: n.applied, emphasis: false },
      { key: 'waiting', label: 'Waiting for a reply', value: n.waiting, emphasis: n.waiting > 0 },
    ],
  }
}
