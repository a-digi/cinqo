import type { Profile } from '../../api'

export function profileLabel(p: Profile): string {
  const name = `${p.firstName} ${p.lastName}`.trim()
  return name || `Profile ${p.id.slice(0, 8)}`
}
