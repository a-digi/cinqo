// Writing the Jobs list's address (step 135): the one place the page changes the address. It takes the window as an
// argument so a script can give it a fake. The address written is always the canonical one for the state (see
// jobsUrlState.ts), on the page's own path, with nothing else carried over.
import { buildJobsSearch, type JobsListState } from './jobsUrlState'

// push adds a history entry (Back returns to the earlier list state); replace changes the current entry (typing in the
// search must not leave one entry per pause).
export type AddressMode = 'push' | 'replace'

export interface AddressTarget {
  location: { pathname: string; search: string }
  history: {
    state: unknown
    pushState: (data: unknown, unused: string, url?: string | URL | null) => void
    replaceState: (data: unknown, unused: string, url?: string | URL | null) => void
  }
}

// Writes the address for the state and reports whether it wrote anything. It acts only while the window is on the list's own
// path (ownPath): the host's router pushes the NEXT page's address before this page is removed, and rewriting that address
// as a list address would destroy where the person is going. An address that already is that address (the same path and
// the same canonical search) is left alone, so no entry is added and nothing is announced. A replace keeps the state
// object the router put on the entry; a push starts a fresh entry.
export function writeJobsAddress(win: AddressTarget, state: JobsListState, mode: AddressMode, ownPath: string): boolean {
  if (win.location.pathname !== ownPath) return false
  const target = ownPath + buildJobsSearch(state)
  if (target === win.location.pathname + win.location.search) return false
  if (mode === 'push') win.history.pushState(null, '', target)
  else win.history.replaceState(win.history.state, '', target)
  return true
}
