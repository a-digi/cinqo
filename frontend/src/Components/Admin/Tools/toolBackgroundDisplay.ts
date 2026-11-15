import type { BackgroundItem, BackgroundRoute } from '../../../api/tools'

// The words and rules of the "Unattended access" block and dialog of a tool card (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 2). Kept out of the
// components so they can be tested on their own. The backend decides in the end: it checks every route again.

export type ItemState = 'approved' | 'pending' | 'blocked'

// approved: an administrator said yes. blocked: the tool it would use no longer allows that route (or is not installed): it cannot be approved, only taken back.
// pending: asked for, allowed, not approved yet.
export const itemState = (i: Pick<BackgroundItem, 'approved' | 'allowedNow'>): ItemState =>
  i.approved ? 'approved' : i.allowedNow ? 'pending' : 'blocked'

export const STATE_WORDS: Record<ItemState, string> = {
  approved: 'Approved',
  pending: 'Not approved yet',
  blocked: 'Not allowed by the tool',
}

// "2 of 3 approved", "Not approved yet", "All approved".
export function summaryText(b: { requested: number; approved: number }): string {
  if (b.approved === 0) return 'Not approved yet'
  if (b.approved >= b.requested) return b.requested === 1 ? 'Approved' : 'All approved'
  return `${String(b.approved)} of ${String(b.requested)} approved`
}

// The tone of the block: amber while something waits for a person, green when everything asked for is approved.
export const summaryTone = (b: { requested: number; approved: number }): 'green' | 'amber' =>
  b.approved >= b.requested ? 'green' : 'amber'

export interface TargetGroup {
  slug: string
  name: string
  items: BackgroundItem[]
}

// The items grouped by the tool they would use, in the order the tools first appear.
export function groupByTarget(items: readonly BackgroundItem[]): TargetGroup[] {
  const groups: TargetGroup[] = []
  for (const i of items) {
    let g = groups.find((x) => x.slug === i.targetSlug)
    if (!g) {
      g = { slug: i.targetSlug, name: i.targetName, items: [] }
      groups.push(g)
    }
    g.items.push(i)
  }
  return groups
}

// What "Approve all that can be approved" sends: every route that is asked for, allowed now and not yet approved.
export const approvableRoutes = (items: readonly BackgroundItem[]): BackgroundRoute[] =>
  items.filter((i) => itemState(i) === 'pending').map(({ targetSlug, method, pathSuffix }) => ({ targetSlug, method, pathSuffix }))

export const routeOf = ({ targetSlug, method, pathSuffix }: BackgroundRoute): BackgroundRoute => ({ targetSlug, method, pathSuffix })

export const routeKey = (r: BackgroundRoute): string => `${r.targetSlug}\x00${r.method}\x00${r.pathSuffix}`

export const dialogTitle = (toolName: string): string => `${toolName}: use without a signed-in person`

export const dialogIntro = (toolName: string): string =>
  `${toolName} asks to use the routes below without a person being signed in, for example to keep watching while nobody has a page open. ` +
  'Approving a route lets it do that for every organisation that switches this on; taking an approval back stops it at once.'

export const approvedBySuffix = (i: Pick<BackgroundItem, 'approvedBy' | 'approvedAt'>): string => {
  if (i.approvedBy === '') return ''
  const at = i.approvedAt.slice(0, 10)
  return at === '' ? ` by ${i.approvedBy}` : ` by ${i.approvedBy} on ${at}`
}
