// repositoryLocalStorage.ts — the persona last chosen, remembered in this browser (a per-viewer convenience, not
// required for correctness). The pages that browse a persona's data take it from their address; this is only the
// fallback for the experience add/edit page when its link names no persona. Each route mounts its own React
// root, so pages share it through the same key, not through shared live state. See
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
