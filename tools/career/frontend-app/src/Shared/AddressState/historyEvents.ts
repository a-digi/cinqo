// Noticing every change of the address (step 135). The browser tells a page about Back and Forward (`popstate`), but
// not about an address the host's router pushes (the sidebar "Jobs" entry while a filtered list is open) nor about one
// the page itself writes with the History API. So the two History functions are wrapped, once, to announce what they
// did: the wrapper runs the original with the same arguments, returns what it returned, and then dispatches an event; a
// failure of the announcement never reaches the caller. Nothing about navigation itself changes.

export const ADDRESS_EVENT = 'cinqo-tool:addresschange'

type HistoryMethod = (data: unknown, unused: string, url?: string | URL | null) => void

// What the functions below need from the window, so a test can give them a fake.
export interface AddressWindow {
  history: { pushState: HistoryMethod; replaceState: HistoryMethod }
  addEventListener: (type: string, listener: () => void) => void
  removeEventListener: (type: string, listener: () => void) => void
  dispatchEvent: (event: Event) => boolean
}

// The History objects already wrapped, so wrapping twice (the page mounting again) never announces twice.
const wrapped = new WeakSet()

function wrap(win: AddressWindow, name: 'pushState' | 'replaceState'): void {
  const original = win.history[name]
  win.history[name] = function (this: unknown, ...args: Parameters<HistoryMethod>): void {
    original.apply(this, args)
    try {
      win.dispatchEvent(new Event(ADDRESS_EVENT))
    } catch {
      // announcing is best effort: navigation has already happened and must not fail because of it
    }
  }
}

// Makes pushState and replaceState announce themselves. Safe to call any number of times.
export function installAddressNotification(win: AddressWindow): void {
  if (wrapped.has(win.history)) return
  wrapped.add(win.history)
  wrap(win, 'pushState')
  wrap(win, 'replaceState')
}

// Calls listener after every change of the address (Back, Forward, pushState, replaceState) and returns the function that
// stops it. Installs the notification first.
export function onAddressChange(win: AddressWindow, listener: () => void): () => void {
  installAddressNotification(win)
  win.addEventListener('popstate', listener)
  win.addEventListener(ADDRESS_EVENT, listener)
  return () => {
    win.removeEventListener('popstate', listener)
    win.removeEventListener(ADDRESS_EVENT, listener)
  }
}
