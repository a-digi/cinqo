import { CV_BUILDER_PATH } from '../../paths'
import { setPersonaIdInLocalStorage } from '../Persona/PersonaSwitcher/repositoryLocalStorage'

// Opens the CV builder to create a CV for one persona. The builder's first step has a persona picker that, when
// it loads, selects the remembered persona (the one saved in this browser) instead of the one in the address, so
// the persona is remembered first: the page then opens on the persona the CV is for. Every "new CV" button goes
// through here.
export function openCvBuilderFor(personaId: string): void {
  setPersonaIdInLocalStorage(personaId)
  window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?personaId=${encodeURIComponent(personaId)}`)
}
