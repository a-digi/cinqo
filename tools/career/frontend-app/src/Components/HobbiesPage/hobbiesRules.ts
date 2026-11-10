// Pure rules of the Hobbies page and form (plan/ai/tools/career/step-231-hobbies-courses-languages.md): the early check of what the server
// checks again, and the count. No React, so they can be script-checked.

import { charCount } from '../../Shared/charCount'

export const MAX_NAME_LENGTH = 200
export const MAX_DESCRIPTION_LENGTH = 2000

// What stops a form from being sent: '' when it can be.
export function formProblem(f: { name: string; description: string }): string {
  const name = f.name.trim()
  if (name === '') return 'The name is required'
  if (charCount(name) > MAX_NAME_LENGTH) return `The name is too long (at most ${String(MAX_NAME_LENGTH)} characters)`
  if (charCount(f.description) > MAX_DESCRIPTION_LENGTH) return `The description is too long (at most ${String(MAX_DESCRIPTION_LENGTH)} characters)`
  return ''
}

// "3 hobbies", "1 hobby".
export function hobbiesCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'hobby' : 'hobbies'}`
}
