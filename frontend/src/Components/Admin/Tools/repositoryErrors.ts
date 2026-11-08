import { ApiError } from '../../../api/client'

// The backend's own message when there is one (a rejected repository file lists
// every problem in it), else a generic fallback.
export function errorText(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}
