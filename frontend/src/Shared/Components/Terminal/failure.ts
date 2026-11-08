import { ApiError } from '../../../api/client'
import type { FileView } from '../../../api/fileReader'
import { describeFailure } from './terminalRules'

// The sentence a failed request becomes in the viewer. The status and the server's own message (fixed phrases, never file text) are used;
// anything that is not an API error is the generic one.
export function failureOf(err: unknown, view: FileView = 'raw'): string {
  if (err instanceof ApiError) return describeFailure(err.status, err.message, view)
  return describeFailure(undefined, undefined, view)
}
