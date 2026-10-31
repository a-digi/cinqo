// Pure rules for the photo library on the Photo page (step 110): the words about who uses
// a photo, and what deleting it costs. No React, so they can be script-checked.

import type { PhotoUser } from '../../api'

const names = (users: PhotoUser[]) => users.map((u) => u.name || '(unnamed)')

// The caption under a photo: nobody, one name, or a count.
export function usedByLabel(users: PhotoUser[]): string {
  if (users.length === 0) return 'Not used'
  if (users.length === 1) return `Used by ${names(users)[0]}`
  return `Used by ${users.length} profiles`
}

// The full list of names, for the tooltip.
export function usedByTitle(users: PhotoUser[]): string {
  return users.length === 0 ? 'No profile uses this photo' : `Used by ${names(users).join(', ')}`
}

// What the delete confirmation says: deleting is for good, and it names how many profiles lose it.
export function deletePhotoMessage(users: PhotoUser[]): string {
  if (users.length === 0) return 'Delete this photo for good? No profile uses it. This cannot be undone.'
  const who = users.length === 1 ? '1 profile uses' : `${users.length} profiles use`
  return `Delete this photo for good? ${who} it and will lose it (${names(users).join(', ')}). This cannot be undone.`
}
