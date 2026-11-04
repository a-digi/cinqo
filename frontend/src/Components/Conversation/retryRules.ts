// The rules around "Retry" on a failed turn (plan/ai/conversation/retry-last-step.md, step 5), kept apart from the components so they can be
// checked on their own. Retry redoes the last step of the failed turn; "Start again" begins a fresh turn.

// After this many failed retries another one asks "are you sure?" first. The server enforces the same number (it answers 409 with the
// code below to a retry that was not confirmed), so this only decides when the dialog is shown before the request is made.
export const CONFIRM_AFTER_FAILED_RETRIES = 3

export const RETRY_CONFIRMATION_CODE = 'retry_confirmation_required'

export function retryNeedsConfirmation(retryCount: number | undefined): boolean {
  return typeof retryCount === 'number' && retryCount >= CONFIRM_AFTER_FAILED_RETRIES
}

// "Retried once" / "Retried 3 times", nothing for a turn that has not been retried (the server leaves the count out for 0).
export function retriedText(retryCount: number | undefined): string | null {
  if (typeof retryCount !== 'number' || !Number.isInteger(retryCount) || retryCount < 1) return null
  return retryCount === 1 ? 'Retried once' : `Retried ${String(retryCount)} times`
}

// The server's answer to a retry that needed a confirmation: HTTP 409 with this code in its JSON body. Anything else (a 409 for a turn that
// is already running, say) is not it.
export function isRetryConfirmationRequired(err: unknown): boolean {
  if (typeof err !== 'object' || err === null) return false
  const { status, body } = err as { status?: unknown; body?: unknown }
  if (status !== 409 || typeof body !== 'object' || body === null) return false
  return (body as { code?: unknown }).code === RETRY_CONFIRMATION_CODE
}
