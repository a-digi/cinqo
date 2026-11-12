// Whether a CV import can start at all (plan: Import CV needs an AI): it reads the CV with an AI, so with no usable AI platform (a remote platform with an
// API key, or the local AI) the upload is not offered and the person is told how to set one up. Pure, so it can be script-checked.

// 'checking' until the platforms and keys were read (the upload stays off, as before, without a message), 'needs-ai' when none is usable, else 'ready'.
export type ImportAiState = 'checking' | 'ready' | 'needs-ai'

export function importAiState(loaded: boolean, usablePlatformCount: number): ImportAiState {
  if (!loaded) return 'checking'
  return usablePlatformCount > 0 ? 'ready' : 'needs-ai'
}

// Shown as the reason on a control that would start an import while no AI is set up.
export const NEEDS_AI_REASON = 'Set up a remote AI (an API key) or a local AI first: the import reads the CV with an AI.'
