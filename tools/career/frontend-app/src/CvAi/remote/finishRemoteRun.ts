import {
  attachCvToRun,
  createCvDocument,
  deleteCvDocument,
  fetchCvAiRuns,
  fetchCvPersonaDefaults,
  fetchCvPhotoStatus,
  type CvAiRun,
  type CvData,
} from '../../api'
import { NO_CV_SAVED_MESSAGE } from '../cvAiRules'
import { decidePhoto, type PhotoDecision } from '../photoDecision'

// How a model-driven run ends (plan 256, step 5). The AI handed its choice over with hand_over_cv (the design, the title and the data, stored on the run); it can never give the CV
// a photo (the photo travels inside the XHTML, which would have to pass through the model twice), so the page makes the CV through the wizard's route, with the profile's photo when
// it is wanted (the profile has one and the run's note does not ask for none), and ties it to the run. A run whose AI handed nothing over ends as failed ("the AI did not create a CV").
export interface FinishRemoteDeps {
  // The run as the server has it now (its choice, its CV), or undefined when it is gone.
  loadRun: (personaId: string, runId: string) => Promise<CvAiRun | undefined>
  // The target role's own data as it is, used when the AI handed no data over (it tailored nothing).
  loadDefaults: (personaId: string) => Promise<CvData>
  hasProfilePhoto: (personaId: string) => Promise<boolean>
  createCv: (personaId: string, templateId: string, title: string, data: CvData, attachPhoto: boolean) => Promise<{ id: string }>
  attach: (runId: string, cvDocumentId: string) => Promise<void>
  deleteCv: (cvDocumentId: string) => Promise<void>
}

export interface FinishedRemoteRun {
  cvDocumentId: string
  // The photo decision, or null when the run already had its CV (another tab, or a run of before this release) and nothing was made.
  photo: PhotoDecision | null
}

export async function finishRemoteCvRun(deps: FinishRemoteDeps, run: { id: string; personaId: string }): Promise<FinishedRemoteRun> {
  const current = await deps.loadRun(run.personaId, run.id)
  if (!current) throw new Error(NO_CV_SAVED_MESSAGE)
  // already made (the old end of the run saved it with save_persona_cv, or another tab finished it): nothing to do
  if (current.cvDocumentId) return { cvDocumentId: current.cvDocumentId, photo: null }
  if (!current.choice) throw new Error(NO_CV_SAVED_MESSAGE)

  // a photo that cannot be looked up counts as no photo: it never stops the CV
  const profileHasPhoto = await deps.hasProfilePhoto(run.personaId).catch(() => false)
  const photo = decidePhoto(profileHasPhoto, current.focus)
  // the AI's tailored data when it handed some over, else the role's own data exactly as it is (step 6): the common case, which costs the AI almost nothing and cannot lose content
  const data = current.choice.data ?? (await deps.loadDefaults(run.personaId))
  const doc = await deps.createCv(run.personaId, current.choice.templateId, current.choice.title, data, photo.attach)
  try {
    await deps.attach(run.id, doc.id)
  } catch (err) {
    // another tab was faster, or the run ended: do not leave a CV that belongs to no run
    await deps.deleteCv(doc.id).catch(() => undefined)
    throw err
  }
  return { cvDocumentId: doc.id, photo }
}

// The real calls.
export function finishRemoteCvRunWithApi(run: { id: string; personaId: string }): Promise<FinishedRemoteRun> {
  return finishRemoteCvRun(
    {
      loadRun: async (personaId, runId) => (await fetchCvAiRuns(personaId)).find((r) => r.id === runId),
      loadDefaults: fetchCvPersonaDefaults,
      hasProfilePhoto: async (personaId) => (await fetchCvPhotoStatus(personaId)).hasPhoto,
      createCv: (personaId, templateId, title, data, attachPhoto) => createCvDocument(personaId, templateId, title, data, attachPhoto),
      attach: attachCvToRun,
      deleteCv: (id) => deleteCvDocument(id),
    },
    run,
  )
}
