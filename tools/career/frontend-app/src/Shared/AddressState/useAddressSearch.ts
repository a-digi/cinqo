import { useRef, useSyncExternalStore } from 'react'
import { onAddressChange } from './historyEvents'

// A page's address as its own query string (the part from "?"), updated after every change of the address: Back and Forward,
// an address the host's router pushes (a sidebar entry), and an address the page writes itself (see writeAddress). Shared by
// the list pages that keep their state in the address (step 135, 136); each page turns the text into its own state with its
// own rules.
//
// `tick` goes up on every such change, even when the text stays the same, so a navigation to the very same address (the
// sidebar entry of the page that is already open) is noticed too.
let addressTick = 0

function subscribe(listener: () => void): () => void {
  return onAddressChange(window, () => {
    addressTick += 1
    listener()
  })
}

// One string, so React sees "nothing changed" between two reads without an event.
function snapshot(): string {
  return `${String(addressTick)}|${window.location.pathname}|${window.location.search}`
}

// ownPath is the path the page is mounted on. While the address is on another path (the host's router pushes the NEXT page's
// address a moment before this page is removed) the value stays what it was, so the page neither reacts to, loads for nor
// rewrites an address that is not its own.
export function useAddressSearch(ownPath: string): { search: string; tick: number } {
  const value = useSyncExternalStore(subscribe, snapshot)
  const [tickText, pathname, ...rest] = value.split('|')
  const last = useRef({ search: '', tick: 0 })
  if (pathname === ownPath) last.current = { search: rest.join('|'), tick: Number(tickText) }
  return last.current
}
