// runLocalResuggest.ts — Re-suggest done in code, for a model that cannot call tools (plan 268, step 4). The code reads the suggestions and the CV, asks the model one small
// tool-free question for each thing the wish concerns (a whole list of skills; or one suggestion at a time), turns the checked answers into operations and puts them on the
// suggestions through the page's route, which keeps them inside the section or rows (the same scope check as a chat's). Whatever the model cannot answer is left as it is and said
// so; nothing is invented. Everything that touches the world is injected, so the flow is testable with a scripted model.
import { runPiece } from '../../../../Cinqo/Conversation/toolFree/askPiece'
import { educationLabel } from '../../educationInsertRules'
import { experienceLabel } from '../../experienceLabel'
import type { CVImportProposal } from '../../parseProposal'
import { cvText } from '../../codeImport/pieces'
import { educationText } from '../../codeImport/pieces'
import { jobDescription, splitCv, type CvJob, type SplitCv } from '../../splitCv'
import { isListSection, type ResuggestScope, type ResuggestSection } from '../resuggestRules'
import { FIELD_SPECS, clip, fieldsPiece, skillsDiffPiece, type FieldSpec } from './localPieces'

// The most rows one local Re-suggest goes through: a small model is slow and each row is a question of its own.
export const MAX_LOCAL_ROWS = 20

export interface ChangeOp {
  op: 'update' | 'add' | 'remove'
  section?: string
  list?: string
  id?: string
  fields?: Record<string, string>
  item?: Record<string, string>
}

export interface LocalResuggestSteps {
  readRun(): Promise<{ proposal: CVImportProposal; revision: string | undefined }>
  readCv(): Promise<string>
  // One question to the model in a conversation with no tools (a fresh turn); resolves with its answer.
  ask(prompt: string): Promise<string>
  // Puts operations on the suggestions inside the scope; rejects with a `status` and the server's sentence.
  apply(ops: ChangeOp[], basedOn: string | undefined): Promise<void>
  progress(text: string): void
}

export interface LocalResuggestResult {
  // One or two plain sentences for the transcript.
  summary: string
  // How many operations were applied.
  applied: number
}

// A refusal of the route carries the HTTP status of the answer.
export function statusOf(err: unknown): number {
  const s = (err as { status?: unknown } | null)?.status
  return typeof s === 'number' ? s : 0
}

const msg = (err: unknown): string => (err instanceof Error ? err.message : String(err))

// The part of the CV that belongs to a job suggestion: the job whose header names the company and whose start date is the same ('' when none is found).
export function jobExcerpt(cv: SplitCv | null, row: { company: string; title: string; startDate: string }): string {
  if (cv === null) return ''
  const company = row.company.toLowerCase().replace(/\s+/g, ' ').trim()
  const job: CvJob | undefined = cv.jobs.find(
    (j) => company !== '' && j.header.toLowerCase().includes(company) && (row.startDate === '' || j.startDate === row.startDate),
  )
  return job ? `${job.header}\n${jobDescription(job)}` : ''
}

type Row = Record<string, string>

// A suggestion as plain text fields (a number such as a minimum salary becomes its text; the model is only shown the fields it may change).
const toRow = (r: object): Row => Object.fromEntries(Object.entries(r).map(([k, v]) => [k, typeof v === 'string' ? v : String(v)]))

// The rows of a list the scope names (all of them for a whole section), at most MAX_LOCAL_ROWS.
function rowsOf(proposal: CVImportProposal, section: ResuggestSection, ids: string[]): { id: string; row: Row; label: string }[] {
  const pick = <T extends { id?: string }>(list: T[], label: (r: T) => string): { id: string; row: Row; label: string }[] =>
    list
      .filter((r) => r.id !== undefined && (ids.length === 0 || ids.includes(r.id)))
      .map((r) => ({ id: r.id ?? '', row: toRow(r), label: label(r) }))
  if (section === 'skills') return pick(proposal.skills, (r) => r.value)
  if (section === 'experience') return pick(proposal.experience, experienceLabel)
  return pick(proposal.education, educationLabel)
}

function singleOf(proposal: CVImportProposal, section: ResuggestSection): Row {
  if (section === 'profile') return toRow(proposal.profile)
  if (section === 'persona') return toRow(proposal.persona)
  return toRow(proposal.personalDetails)
}

const asText = (row: Row, specs: FieldSpec[]): Record<string, string> => Object.fromEntries(specs.map((s) => [s.key, row[s.key] ?? '']))

export async function runLocalResuggest(scope: ResuggestScope, wish: string, steps: LocalResuggestSteps): Promise<LocalResuggestResult> {
  const { proposal, revision } = await steps.readRun()
  let cv: SplitCv | null = null
  try {
    const parsed = splitCv(await steps.readCv())
    if (parsed.usable) cv = parsed
  } catch {
    // the suggestions are still there to work from
  }
  const ask = (prompt: string): Promise<string> => steps.ask(prompt)
  const ops: ChangeOp[] = []
  const left: string[] = []
  const section = scope.section

  if (!isListSection(section)) {
    steps.progress('Looking at the suggestion…')
    const specs = FIELD_SPECS[section as 'profile' | 'persona' | 'personalDetails']
    const excerpt = section === 'profile' ? (cv?.header.join('\n') ?? '') : cv ? clip(cvText(cv), 3000) : ''
    const o = await runPiece(fieldsPiece(specs, asText(singleOf(proposal, section), specs), excerpt, wish), ask)
    if (o.value === null) left.push(`the suggestion (${o.note || 'no usable answer'})`)
    else if (Object.keys(o.value).length > 0) {
      ops.push({ op: 'update', section, fields: o.value })
    }
  } else if (section === 'skills' && scope.itemIds.length === 0) {
    steps.progress('Looking at the skills…')
    const rows = rowsOf(proposal, 'skills', [])
    const o = await runPiece(
      skillsDiffPiece(
        rows.map((r) => r.row.value),
        cv ? skillsExcerpt(cv) : '',
        wish,
      ),
      ask,
    )
    if (o.value === null) left.push(`the skills (${o.note || 'no usable answer'})`)
    else {
      for (const value of o.value.remove) {
        const hit = rows.find(
          (r) => r.row.value.toLowerCase().replace(/\s+/g, ' ').trim() === value.toLowerCase().replace(/\s+/g, ' ').trim(),
        )
        if (hit) ops.push({ op: 'remove', list: 'skills', id: hit.id })
      }
      for (const value of o.value.add) ops.push({ op: 'add', list: 'skills', item: { value } })
    }
  } else {
    const all = rowsOf(proposal, section, scope.itemIds)
    const rows = all.slice(0, MAX_LOCAL_ROWS)
    if (all.length > rows.length) left.push(`${String(all.length - rows.length)} more rows (at most ${String(MAX_LOCAL_ROWS)} at a time)`)
    const specs = FIELD_SPECS[section as 'skills' | 'experience' | 'education']
    for (const [i, r] of rows.entries()) {
      steps.progress(
        rows.length === 1 ? 'Looking at the suggestion…' : `Looking at ${String(i + 1)} of ${String(rows.length)}: ${clip(r.label, 50)}`,
      )
      const excerpt =
        section === 'experience'
          ? jobExcerpt(cv, { company: r.row.company, title: r.row.title, startDate: r.row.startDate })
          : section === 'education' && cv
            ? educationText(cv)
            : ''
      const o = await runPiece(fieldsPiece(specs, asText(r.row, specs), excerpt, wish), ask)
      if (o.value === null) left.push(`${r.label} (${o.note || 'no usable answer'})`)
      else if (Object.keys(o.value).length > 0) {
        ops.push({ op: 'update', list: section, id: r.id, fields: o.value })
      }
    }
  }

  if (ops.length === 0) {
    return { summary: leftSentence('Nothing needed changing.', left), applied: 0 }
  }
  steps.progress('Saving the changes…')
  let applied = ops.length
  try {
    await steps.apply(ops, revision)
  } catch (err) {
    // The suggestions moved meanwhile, or the run is closed: nothing was changed and the person is told why.
    if (statusOf(err) !== 400) throw err
    // The server refused something in the batch: try the operations one by one so that the good ones are kept.
    applied = 0
    for (const op of ops) {
      try {
        await steps.apply([op], undefined)
        applied++
      } catch (one) {
        if (statusOf(one) !== 400) throw one
        left.push(`${opLabel(op)} (${msg(one)})`)
      }
    }
  }
  const head = applied === 0 ? 'Nothing could be changed.' : `Changed ${String(applied)} ${applied === 1 ? 'thing' : 'things'}.`
  return { summary: leftSentence(head, left), applied }
}

function opLabel(op: ChangeOp): string {
  return op.op === 'add'
    ? `adding ${op.item?.value ?? 'a row'}`
    : op.op === 'remove'
      ? 'a removal'
      : `a change to ${op.list ?? op.section ?? 'a suggestion'}`
}

function leftSentence(head: string, left: string[]): string {
  return left.length === 0 ? head : `${head} Left as it was: ${left.join('; ')}.`
}

function skillsExcerpt(cv: SplitCv): string {
  return cv.sections
    .filter((s) => s.kind === 'skills')
    .flatMap((s) => s.lines)
    .join('\n')
}
