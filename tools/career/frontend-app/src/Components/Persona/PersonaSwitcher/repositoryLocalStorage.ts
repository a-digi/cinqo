// repositoryLocalStorage.ts — PersonaSwitcher's own current-persona-id
// localStorage repository (moved out of the tool-wide personaStore.ts,
// step 50, once PersonaSwitcher turned out to be its only caller): a
// per-viewer convenience (not required for correctness: every page
// re-resolves an "effective" persona id on its own mount via
// PersonaSwitcher, falling back to the first available persona if the
// stored one is stale or missing). Each route mounts its own
// independent React root (no shared parent layout across routes), so
// this is how pages stay in sync with each other — same key, not
// shared live state. See
// plan/ai/tools/career/step-09-persona-frontend.md.
const STORAGE_KEY = 'career.currentPersonaId'

export function getPersonaIdFromLocalStorage(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

export function setPersonaIdInLocalStorage(id: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, id)
  } catch {
    // per-viewer convenience only — never required for correctness
  }
}
