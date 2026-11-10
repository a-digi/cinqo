// The Applications item of the sidebar shows only when there are applications (plan/ai/tools/career/step-238-applications-menu-item-only-with-applications.md).
// The entry is registered hidden (menu.ts), so it never flashes; this module asks the server how many applications there are and shows or hides
// the item through the bridge. It runs when the bundle loads and every time a Career page opens (main.tsx), one small request each; a request that
// fails changes nothing (the item keeps the state it had, hidden at the start).
import { APPLICATIONS_PATH } from './paths'

// The count in the server's answer ({ total: N }), or null when it is not that shape (an older or newer backend): then nothing changes.
export function parseApplicationCount(raw: unknown): number | null {
  if (typeof raw !== 'object' || raw === null) return null
  const total = (raw as { total?: unknown }).total
  return typeof total === 'number' && Number.isInteger(total) && total >= 0 ? total : null
}

export interface ApplicationsMenuDeps {
  fetchCount: () => Promise<unknown>
  // The bridge's setMenuEntryHidden, or undefined on a host that does not have it (the item then simply stays visible).
  setHidden: ((path: string, hidden: boolean) => void) | undefined
}

// refresh() asks once and applies the answer. While a request is running, further calls do not start another: they make one more request run right
// after it (so a change made while the first request was in flight is not missed), and all of them wait for the same promise.
export function createApplicationsMenuRefresher(deps: ApplicationsMenuDeps): () => Promise<void> {
  let running: Promise<void> | null = null
  let again = false
  // read through a function: a call made during the await sets it, which the compiler cannot see from the assignment above it
  const askedAgain = () => again

  async function once(): Promise<void> {
    if (deps.setHidden === undefined) return
    try {
      const total = parseApplicationCount(await deps.fetchCount())
      if (total !== null) deps.setHidden(APPLICATIONS_PATH, total === 0)
    } catch {
      // keep the state it had
    }
  }

  return function refresh(): Promise<void> {
    if (running !== null) {
      again = true
      return running
    }
    running = (async () => {
      do {
        again = false
        await once()
      } while (askedAgain())
    })().finally(() => {
      running = null
    })
    return running
  }
}
