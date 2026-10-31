import { cvBuilderPathFor } from '../../paths'

// Opens the CV builder to create a CV for one persona, on that persona's own address (the one every "new CV"
// button goes through).
export function openCvBuilderFor(personaId: string): void {
  window.__cinqoToolBridge.navigate(cvBuilderPathFor(personaId))
}
