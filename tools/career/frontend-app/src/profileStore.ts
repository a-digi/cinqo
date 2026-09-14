// profileStore.ts — the current profile (job seeker) id, kept in
// localStorage exactly like personaStore.ts's own current-persona-id
// — a per-viewer convenience, not required for correctness. See
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
const STORAGE_KEY = 'career.currentProfileId'

export function getStoredProfileId(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

export function setStoredProfileId(id: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, id)
  } catch {
    // per-viewer convenience only — never required for correctness
  }
}
