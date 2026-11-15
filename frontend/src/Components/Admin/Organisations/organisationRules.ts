import type { Organisation, OrganisationStatus } from '../../../api/organisations'

// The rules of the Organisations page, kept out of the component so they can be tested on their own. They mirror the backend's (api/src/organisation):
// a name is 1 to 120 characters, a user id 1 to 200, and the backend decides in the end.

export const NAME_MAX = 120
export const USER_ID_MAX = 200

export const ORGANISATIONS_PATH = '/admin/organisations'

export const cleanName = (s: string): string => s.trim()

export const validName = (s: string): boolean => {
  const n = Array.from(cleanName(s)).length // characters, like the backend (not UTF-16 units)
  return n >= 1 && n <= NAME_MAX
}

export const validUserId = (s: string): boolean => {
  const t = s.trim()
  return t.length >= 1 && t.length <= USER_ID_MAX
}

export const STATUS_WORDS: Record<OrganisationStatus, string> = { active: 'Active', suspended: 'Suspended' }

export const memberText = (n: number): string => (n === 1 ? '1 user' : `${String(n)} users`)

export const otherStatus = (s: OrganisationStatus): OrganisationStatus => (s === 'active' ? 'suspended' : 'active')

// The words on the button that flips the status, and what the person is asked before it is flipped (suspending cuts off the whole organisation).
export const flipWords = (o: Pick<Organisation, 'status' | 'name' | 'memberCount'>): { button: string; title: string; message: string } =>
  o.status === 'active'
    ? {
        button: 'Suspend',
        title: 'Suspend organisation',
        message: `Suspend "${o.name}"? Its ${memberText(o.memberCount)} will get no access to any tool until it is active again.`,
      }
    : { button: 'Activate', title: 'Activate organisation', message: `Activate "${o.name}"? Its users get their tool access back.` }

// The operator's own organisation cannot be suspended (it would lock them out; the backend refuses it too).
export const canSuspend = (o: Pick<Organisation, 'id' | 'status'>, ownOrganisationId: string | undefined): boolean =>
  o.status === 'suspended' || o.id !== ownOrganisationId

export const sameName = (a: string, b: string): boolean => cleanName(a) === cleanName(b)
