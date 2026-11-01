import { useMemo, useRef, useSyncExternalStore } from 'react'
import { onAddressChange } from './historyEvents'
import { parseJobsUrl, type JobsListState } from './jobsUrlState'

// The Jobs list's state as the address says it (step 135), updated after every change of the address: Back and Forward,
// an address the host's router pushes (the sidebar entry), and an address the page writes itself. `tick` goes up on
// every such change, even when the address stays the same text, so a navigation to the very same address (the sidebar
// "Jobs" entry on an open list) is noticed too.
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

// The Jobs list's state as the address says it. While the address is on another path (the host's router pushes the next
// page's address a moment before this page is removed) the value stays what it was, so the page does not react to, load
// for or rewrite an address that is not its own.
export function useJobsUrl(ownPath: string): { state: JobsListState; tick: number } {
  const value = useSyncExternalStore(subscribe, snapshot)
  const [tickText, pathname, ...rest] = value.split('|')
  const search = rest.join('|')
  const own = pathname === ownPath
  const state = useMemo(() => parseJobsUrl(own ? search : ''), [own, search])
  const last = useRef({ state, tick: Number(tickText) })
  if (own) last.current = { state, tick: Number(tickText) }
  return last.current
}
