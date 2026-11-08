// repositoryLocalStorage.ts — the profile last chosen, remembered in this browser (a per-viewer convenience, not
// required for correctness). The pages that browse a profile's data take it from their address; this is only the
// profile the "new persona" form starts on when its link names none. See
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
