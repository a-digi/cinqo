import type { BackgroundOverview, Organisation } from '../../../api/organisations'

// The words and rules of the unattended-use parts of the Organisations page (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 4).

export const PERIODS = [7, 30, 90] as const
export const DEFAULT_PERIOD = 7

export const periodText = (days: number): string => `the last ${String(days)} days`

// How an organisation's switch reads and what the button next to it does.
export const switchWords = (o: Pick<Organisation, 'backgroundEnabled'>): { state: 'On' | 'Off'; action: 'Turn on' | 'Turn off' } =>
  o.backgroundEnabled ? { state: 'On', action: 'Turn off' } : { state: 'Off', action: 'Turn on' }

// Turning it ON asks first: it lets approved tools work for the organisation while nobody is signed in. Turning it off is safe and happens at once.
export const turnOnQuestion = (name: string): string =>
  `Turn on unattended use for "${name}"? Tools that were approved on the Tools page may then work for its users while nobody is signed in, for example to keep watching a live stream.`

export const switchedText = (name: string, enabled: boolean): string => `Unattended use is now ${enabled ? 'on' : 'off'} for "${name}".`

export const KILL_SWITCH_TEXT =
  'Unattended use is switched off for the whole app (CINQO_BACKGROUND=off). No tool can work without a signed-in person until that is removed and Cinqo is restarted.'

export function totals(rows: BackgroundOverview['usage']): { calls: number; denied: number } {
  let calls = 0
  let denied = 0
  for (const r of rows) {
    calls += r.calls
    denied += r.denied
  }
  return { calls, denied }
}

// "2 of 3 approved" for a tool on the overview ("none approved" when nothing is, "all 3 approved" when everything is).
export function approvalText(t: { requested: number; approved: number }): string {
  if (t.approved === 0) return t.requested === 1 ? 'not approved' : `none of ${String(t.requested)} approved`
  if (t.approved >= t.requested) return t.requested === 1 ? 'approved' : `all ${String(t.requested)} approved`
  return `${String(t.approved)} of ${String(t.requested)} approved`
}

// An organisation name for a usage row: the name, or a plain word when the tool did not name a usable one.
export const orgLabel = (r: { orgId: string; orgName: string }): string => (r.orgId === '' ? 'no organisation named' : r.orgName)
