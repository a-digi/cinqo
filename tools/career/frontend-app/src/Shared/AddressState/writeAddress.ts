// Writing a page's address (step 135, 136): the one way a list page changes it. It takes the window as an argument so a
// script can give it a fake, and a canonical query string (what the page's own rules build: "" or "?a=1&b=2") so it works
// for every page.

// push adds a history entry (Back returns to the earlier state); replace changes the current entry (typing in a search must
// not leave one entry per pause).
export type AddressMode = 'push' | 'replace'

export interface AddressTarget {
  location: { pathname: string; search: string }
  history: {
    state: unknown
    pushState: (data: unknown, unused: string, url?: string | URL | null) => void
    replaceState: (data: unknown, unused: string, url?: string | URL | null) => void
  }
}

// Makes the address ownPath + search and reports whether it wrote anything. It acts only while the window is on the page's
// own path: the host's router pushes the NEXT page's address a moment before this page is removed, and rewriting that
// address would destroy where the person is going. An address that already is that address is left alone, so no entry is
// added and nothing is announced. A replace keeps the state object the router put on the entry; a push starts a fresh entry.
export function writeAddress(win: AddressTarget, ownPath: string, search: string, mode: AddressMode): boolean {
  if (win.location.pathname !== ownPath) return false
  const target = ownPath + search
  if (target === win.location.pathname + win.location.search) return false
  if (mode === 'push') win.history.pushState(null, '', target)
  else win.history.replaceState(win.history.state, '', target)
  return true
}
