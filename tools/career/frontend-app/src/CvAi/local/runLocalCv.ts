import type { CvData, CvDocument, CvTemplate } from '../../api'
import { runPiece, type PieceOutcome } from '../../Cinqo/Conversation/toolFree/askPiece'
import { candidateTemplates, templatePiece } from './chooseTemplate'

// The local driver of "Create a CV with AI" (plan 253): for a small local model that cannot follow the long tool prompt (Phi-4-mini). Code does the work; the model is asked
// one small question with no tools, which design suits the person. Code reads the target role's data (the CV is exactly that data: nothing reordered, tightened or
// invented), makes the CV through the same route as the wizard, and ties it to the run.
export interface LocalCvDeps {
  loadDefaults: (personaId: string) => Promise<CvData>
  loadTemplates: () => Promise<CvTemplate[]>
  // One question as a fresh turn of the run's own conversation, no tools; resolves with the model's answer.
  ask: (prompt: string) => Promise<string>
  createCv: (personaId: string, templateId: string, title: string, data: CvData) => Promise<Pick<CvDocument, 'id'>>
  attach: (runId: string, cvDocumentId: string) => Promise<void>
}

export interface LocalCvResult {
  cvDocumentId: string
  templateId: string
  // Who chose the design: the model, or code (the model gave no usable answer twice).
  source: PieceOutcome<string>['source']
  note: string
  // One plain sentence for the run's last message.
  summary: string
}

export async function runLocalCv(
  deps: LocalCvDeps,
  persona: { id: string; name: string },
  focus: string,
  runId: string,
): Promise<LocalCvResult> {
  const data = await deps.loadDefaults(persona.id)
  const templates = await deps.loadTemplates()
  const candidates = candidateTemplates(templates, `${persona.name} ${data.headline} ${focus}`)
  if (candidates.length === 0) throw new Error('No CV design is available.')

  const outcome = await runPiece(
    templatePiece({ name: data.fullName || persona.name, headline: data.headline, focus }, candidates),
    deps.ask,
  )
  const templateId = outcome.value
  // The piece always has a fallback while there are candidates, so a value is there; this keeps the type honest.
  if (templateId === null) throw new Error('No CV design could be chosen.')

  const title = `${persona.name || data.fullName} - CV`
  const doc = await deps.createCv(persona.id, templateId, title, data)
  await deps.attach(runId, doc.id)

  const name = candidates.find((c) => c.id === templateId)?.name ?? templateId
  const summary =
    outcome.source === 'model'
      ? `I created your CV with the ${name} template.`
      : `I created your CV with the ${name} template (the AI's own choice could not be used).`
  return { cvDocumentId: doc.id, templateId, source: outcome.source, note: outcome.note, summary }
}
