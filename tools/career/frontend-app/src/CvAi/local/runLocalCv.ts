import type { CvData, CvDocument, CvTemplate } from '../../api'
import { runPiece, type PieceOutcome } from '../../Cinqo/Conversation/toolFree/askPiece'
import { candidateTemplates, templatePiece, type CandidateMode } from './chooseTemplate'
import { decidePhoto, wantsAts, type PhotoDecision } from '../photoDecision'

// The local driver of "Create a CV with AI" (plan 253): for a small local model that cannot follow the long tool prompt (Phi-4-mini). Code does the work; the model is asked
// one small question with no tools, which design suits the person. Code reads the target role's data (the CV is exactly that data: nothing reordered, tightened or
// invented), decides whether the CV carries the profile's photo (plan 256: it does when the profile has one, unless the note asks for none), makes the CV through the same
// route as the wizard, and ties it to the run.
export interface LocalCvDeps {
  loadDefaults: (personaId: string) => Promise<CvData>
  loadTemplates: () => Promise<CvTemplate[]>
  // Whether the target role's profile has a photo. A failure counts as "no photo": a photo that cannot be looked up must not stop the CV.
  hasProfilePhoto: (personaId: string) => Promise<boolean>
  // One question as a fresh turn of the run's own conversation, no tools; resolves with the model's answer.
  ask: (prompt: string) => Promise<string>
  createCv: (personaId: string, templateId: string, title: string, data: CvData, attachPhoto: boolean) => Promise<Pick<CvDocument, 'id'>>
  attach: (runId: string, cvDocumentId: string) => Promise<void>
}

export interface LocalCvResult {
  cvDocumentId: string
  templateId: string
  // Who chose the design: the model, or code (the model gave no usable answer twice).
  source: PieceOutcome<string>['source']
  note: string
  photo: PhotoDecision
  // One plain sentence for the run's last message.
  summary: string
}

function modeFor(photo: PhotoDecision, focus: string): CandidateMode {
  if (!photo.attach) return 'no-photo'
  return wantsAts(focus) ? 'photo-ats' : 'photo'
}

const PHOTO_WORDS: Record<PhotoDecision['reason'], string> = {
  photo: ' with your photo',
  'asked-none': ' without a photo, as you asked',
  'no-profile-photo': ' without a photo (your profile has none)',
}

export async function runLocalCv(
  deps: LocalCvDeps,
  persona: { id: string; name: string },
  focus: string,
  runId: string,
): Promise<LocalCvResult> {
  const data = await deps.loadDefaults(persona.id)
  const templates = await deps.loadTemplates()
  const profileHasPhoto = await deps.hasProfilePhoto(persona.id).catch(() => false)
  const photo = decidePhoto(profileHasPhoto, focus)
  const mode = modeFor(photo, focus)
  const candidates = candidateTemplates(templates, `${persona.name} ${data.headline} ${focus}`, mode)
  if (candidates.length === 0) throw new Error('No CV design is available.')

  const outcome = await runPiece(
    templatePiece({ name: data.fullName || persona.name, headline: data.headline, focus, photo: photo.attach }, candidates, mode),
    deps.ask,
  )
  const templateId = outcome.value
  // The piece always has a fallback while there are candidates, so a value is there; this keeps the type honest.
  if (templateId === null) throw new Error('No CV design could be chosen.')

  const title = `${persona.name || data.fullName} - CV`
  const doc = await deps.createCv(persona.id, templateId, title, data, photo.attach)
  await deps.attach(runId, doc.id)

  const name = candidates.find((c) => c.id === templateId)?.name ?? templateId
  const choice = outcome.source === 'model' ? '' : " (the AI's own choice could not be used)"
  const summary = `I created your CV with the ${name} template${PHOTO_WORDS[photo.reason]}${choice}.`
  return { cvDocumentId: doc.id, templateId, source: outcome.source, note: outcome.note, photo, summary }
}
