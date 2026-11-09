// runCodeImport.ts — the CV import done in code, for a model that cannot take a whole CV (plan 209, step 4).
//
// The code reads the CV, cuts it into its parts (splitCv), asks the model one small question for each thing it cannot read by itself (no
// tools, only text in and JSON out, each answer checked against the CV: pieces.ts), puts every piece on the run's draft through the same
// checks the cv_import_* tools use, and finishes the draft. Whatever the model cannot answer is read by the code or left out; nothing is
// invented. Everything that touches the world is injected (ask, store, finish), so the whole flow is testable with a scripted model.

import { jobDescription, splitCv, type CvJob, type SplitCv } from '../splitCv'
import { runPiece, type PieceOutcome } from './askPiece'
import {
  companyTitlePiece,
  detailsPiece,
  educationFromDatedLines,
  educationFromLines,
  educationPiece,
  educationText,
  nameFromHeader,
  namePiece,
  personaPiece,
  skillsPiece,
  skillsText,
  type CompanyTitle,
} from './pieces'

export interface CodeImportSteps {
  // One question to the model in a conversation with no tools; resolves with its answer.
  ask(prompt: string): Promise<string>
  // Puts one piece on the draft; rejects with the server's reason when it is refused.
  store(section: string, body: { value?: unknown; items?: unknown }): Promise<void>
  // Completes the draft into the proposal; rejects with what is missing.
  finish(): Promise<void>
}

// What came of each piece, so a weak result can be explained.
export interface PieceNote {
  piece: string
  source: 'model' | 'code' | 'none'
  note: string
}

export interface CodeImportResult {
  notes: PieceNote[]
}

export const NAME_NOT_FOUND_MESSAGE = "The person's name could not be found in the CV."

// The most jobs one import sends: the backend takes 50.
const MAX_JOBS = 50

export type CodePrep = { ok: true; cv: SplitCv } | { ok: false; reason: string }

// Reads and cuts the CV. Not ok when the code cannot work from it (the text cannot be read, no heading it knows, neither jobs nor skills): the
// caller then runs the step-by-step import instead.
export async function prepareCodeImport(readText: () => Promise<string>): Promise<CodePrep> {
  let text: string
  try {
    text = await readText()
  } catch (err) {
    return { ok: false, reason: `the CV text could not be read (${err instanceof Error ? err.message : String(err)})` }
  }
  const cv = splitCv(text)
  if (!cv.usable) return { ok: false, reason: 'the CV has no heading the code knows (profile, skills, experience or education)' }
  if (cv.jobs.length === 0 && skillsText(cv).trim() === '') return { ok: false, reason: 'the code found neither jobs nor skills in the CV' }
  // A job whose header line the code could not find would be stored as "Not stated": better the step-by-step import than a proposal with holes.
  if (cv.jobs.some((j) => j.header === ''))
    return { ok: false, reason: 'the code could not tell the header line (company and title) of every job' }
  return { ok: true, cv }
}

const insert = { action: 'insert', possibleDuplicateOf: null }
const skip = { action: 'skip', possibleDuplicateOf: null }

export async function runCodeImport(cv: SplitCv, steps: CodeImportSteps): Promise<CodeImportResult> {
  const notes: PieceNote[] = []
  const ask = (prompt: string): Promise<string> => steps.ask(prompt)
  const note = (piece: string, source: PieceNote['source'], text = ''): void => {
    notes.push({ piece, source, note: text })
  }
  const asked = async <T>(piece: string, run: () => Promise<PieceOutcome<T>>): Promise<T | null> => {
    const o = await run()
    note(piece, o.source, o.note)
    return o.value
  }

  // The name: read by code from the top lines, and asked only when no line looks like one.
  let name = nameFromHeader(cv.header)
  if (name === null) name = await asked('name', () => runPiece(namePiece(cv), ask))
  else note('name', 'code')
  if (name === null) throw new Error(NAME_NOT_FOUND_MESSAGE)
  await steps.store('profile', { value: { ...insert, name } })

  const persona = await asked('persona', () => runPiece(personaPiece(cv), ask))
  await steps.store('persona', {
    value: persona ? { ...insert, name: persona.name, title: persona.title } : { ...skip, name: '', title: '' },
  })

  const details = await asked('personal details', () => runPiece(detailsPiece(cv), ask))
  await steps.store('personalDetails', {
    value: details
      ? { ...insert, headline: details.headline, summary: details.summary, location: details.location }
      : { ...skip, headline: '', summary: '', location: '' },
  })

  const skills =
    skillsText(cv).trim() === '' && cv.jobs.every((j) => j.technologies === '')
      ? null
      : await asked('skills', () => runPiece(skillsPiece(cv), ask))
  if (skills) await steps.store('skills', { items: skills.map((value) => ({ ...insert, value })) })

  let n = 0
  for (const job of cv.jobs.slice(0, MAX_JOBS)) {
    n++
    await storeJob(job, n, steps, ask, note)
  }

  const lines = educationText(cv)
    .split('\n')
    .filter((l) => l.trim() !== '')
  if (lines.length > 0) {
    const byCode = educationFromLines(lines) ?? educationFromDatedLines(lines)
    const entries = byCode ?? (await asked('education', () => runPiece(educationPiece(cv), ask)))
    if (byCode) note('education', 'code')
    if (entries && entries.length > 0) {
      try {
        await steps.store('education', { items: entries.map((e) => ({ ...insert, ...e, description: '' })) })
      } catch (err) {
        note('education', 'none', `the server refused it: ${err instanceof Error ? err.message : String(err)}`)
      }
    }
  }

  await steps.finish()
  return { notes }
}

// One job: company and title by code when the CV makes them certain, else asked; its description built from the CV's own lines.
async function storeJob(
  job: CvJob,
  n: number,
  steps: CodeImportSteps,
  ask: (prompt: string) => Promise<string>,
  note: (piece: string, source: PieceNote['source'], text?: string) => void,
): Promise<void> {
  const label = `job ${String(n)}`
  let ct: CompanyTitle | null = job.company !== null && job.title !== null ? { company: job.company, title: job.title } : null
  if (ct) note(label, 'code')
  else if (job.header !== '') {
    const o = await runPiece(companyTitlePiece(job.header), ask)
    note(label, o.source, o.note)
    ct = o.value
  }
  ct ??= { company: 'Not stated', title: 'Not stated' }
  const base = {
    ...insert,
    company: ct.company,
    title: ct.title,
    startDate: job.startDate,
    endDate: job.endDate,
    description: jobDescription(job),
  }
  try {
    await steps.store('experience', { value: { ...base, bulletsInCv: job.bullets.length } })
  } catch (first) {
    // The bullet count is what the server checks the description against: once more without it before leaving the job out.
    try {
      await steps.store('experience', { value: base })
      note(label, 'code', `stored without the bullet count (${first instanceof Error ? first.message : String(first)})`)
    } catch (second) {
      note(label, 'none', `the server refused it: ${second instanceof Error ? second.message : String(second)}`)
    }
  }
}
