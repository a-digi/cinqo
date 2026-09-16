// repositoryLocalStorage.ts — ProfileSwitcher's own current-profile-id
// localStorage repository (moved out of the tool-wide profileStore.ts,
// step 51, once ProfileSwitcher turned out to be its only caller —
// same move as step 50's own personaStore.ts ->
// Persona/PersonaSwitcher/repositoryLocalStorage.ts): a per-viewer
// convenience, not required for correctness. See
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
const STORAGE_KEY = 'career.currentProfileId'

export function getProfileIdFromLocalStorage(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

export function setProfileIdInLocalStorage(id: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, id)
  } catch {
    // per-viewer convenience only — never required for correctness
  }
}
