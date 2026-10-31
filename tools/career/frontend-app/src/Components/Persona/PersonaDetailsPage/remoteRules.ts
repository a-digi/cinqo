// The remote-work preference of a persona (step 130): pure rules for the Summary & Goals control, kept apart from the
// component so they can be checked on their own. The values are the backend's.
export type RemotePreference = '' | 'onsite' | 'open' | 'remote_only'

export const REMOTE_OPTIONS: { value: RemotePreference; label: string; hint: string }[] = [
  { value: '', label: 'Not set', hint: 'The job match reads a place, and the word “remote” in your desired locations, as it always did.' },
  { value: 'onsite', label: 'On-site only', hint: 'Only the job’s place counts; a remote job does not help.' },
  {
    value: 'open',
    label: 'Open to remote or hybrid',
    hint: 'A remote job fits wherever it is; an on-site or hybrid job must be in a place you want.',
  },
  { value: 'remote_only', label: 'Remote only', hint: 'Any remote job fits; an on-site job never does, even in your own city.' },
]

// What the server sent, as a value the form knows: anything else (an older server, a bad value) is "not set".
export function asRemotePreference(value: unknown): RemotePreference {
  return value === 'onsite' || value === 'open' || value === 'remote_only' ? value : ''
}

export function remoteLabel(value: RemotePreference): string {
  return REMOTE_OPTIONS.find((o) => o.value === value)?.label ?? 'Not set'
}
