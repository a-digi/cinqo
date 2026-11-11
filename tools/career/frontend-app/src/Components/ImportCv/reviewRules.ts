// Pure rules of the import review (moved out of ImportCvPage in plan 166, step 3): the checklist's first state and the save
// summary that is stored on the run after every insert. No React, so they can be script-checked.
import { isInitiallyChecked, type CVImportProposal } from './parseProposal'
import type { InsertResult, ItemResult } from './insertProposal'
import { experienceLabel } from './experienceLabel'
import { educationLabel } from './educationInsertRules'

// buildFullSaveSummary reconstructs the InsertResult shape
// cv_import_runs.go's own save_summary_json stores, from this
// component's own accumulated insertedKeys/insertErrors state — not
// from insertSelected's own per-call return value, which only ever
// covers items attempted in THAT call. Takes the two maps as explicit
// params (not read from component state) so a caller can pass the
// just-computed next state before it's actually committed via
// setState — React state updates aren't synchronously readable
// immediately after being requested.
// The key of a row in the review's state (which rows are ticked, inserted, failed): by the row's own id once the server gave it one (plan 257), so an add, a remove or an
// update made in the chat does not move the state of the other rows; by position for a proposal that has no ids yet (older runs the AI never touched).
export type RowKind = 'skill' | 'experience' | 'education'

export function rowKey(kind: RowKind, item: { id?: string }, index: number): string {
  return item.id ? `${kind}:${item.id}` : `${kind}-${index}`
}

// buildFullSaveSummary reconstructs the InsertResult shape
// cv_import_runs.go's own save_summary_json stores, from this
// component's own accumulated insertedKeys/insertErrors state — not
// from insertSelected's own per-call return value, which only ever
// covers items attempted in THAT call. Takes the two maps as explicit
// params (not read from component state) so a caller can pass the
// just-computed next state before it's actually committed via
// setState — React state updates aren't synchronously readable
// immediately after being requested. Every row entry carries its index and, when the row has one, its id (plan 257): readers of an older summary use the index, newer ones the id.
export function buildFullSaveSummary(
  p: CVImportProposal,
  insertedKeys: Record<string, boolean>,
  insertErrors: Record<string, string>,
  // What a row says besides "inserted" (an entry the profile already had), by key.
  insertedNotes: Record<string, string> = {},
): InsertResult {
  function itemResult(key: string): ItemResult | undefined {
    if (insertedKeys[key]) return { ok: true }
    if (insertErrors[key]) return { ok: false, error: insertErrors[key] }
    return undefined
  }
  const idOf = (item: { id?: string }) => (item.id ? { id: item.id } : {})
  const skills: (ItemResult & { index: number; value: string; id?: string })[] = []
  p.skills.forEach((s, i) => {
    const r = itemResult(rowKey('skill', s, i))
    if (r) skills.push({ ...r, ...idOf(s), index: i, value: s.value })
  })
  const experience: (ItemResult & { index: number; label: string; id?: string })[] = []
  p.experience.forEach((e, i) => {
    const r = itemResult(rowKey('experience', e, i))
    if (r) experience.push({ ...r, ...idOf(e), index: i, label: experienceLabel(e) })
  })
  const education: (ItemResult & { index: number; label: string; id?: string })[] = []
  p.education.forEach((e, i) => {
    const key = rowKey('education', e, i)
    const r = itemResult(key)
    if (r)
      education.push({ ...r, ...(insertedNotes[key] ? { note: insertedNotes[key] } : {}), ...idOf(e), index: i, label: educationLabel(e) })
  })
  return {
    profile: itemResult('profile'),
    persona: itemResult('persona'),
    personalDetails: itemResult('personalDetails'),
    skills,
    experience,
    education,
  }
}

export function buildInitialChecked(p: CVImportProposal): Record<string, boolean> {
  const checked: Record<string, boolean> = {
    profile: isInitiallyChecked(p.profile),
    persona: isInitiallyChecked(p.persona),
    personalDetails: isInitiallyChecked(p.personalDetails),
  }
  p.skills.forEach((s, i) => {
    checked[rowKey('skill', s, i)] = isInitiallyChecked(s)
  })
  p.experience.forEach((e, i) => {
    checked[rowKey('experience', e, i)] = isInitiallyChecked(e)
  })
  p.education.forEach((e, i) => {
    checked[rowKey('education', e, i)] = isInitiallyChecked(e)
  })
  return checked
}

// ---- the AI changed the suggestions while the person was reviewing (plan 257) ----------------------------------------------------------------

// What the review remembers about the rows: ticked, inserted, the last error and a note, each by key.
export interface ReviewState {
  checked: Record<string, boolean>
  insertedKeys: Record<string, boolean>
  insertErrors: Record<string, string>
  insertNotes: Record<string, string>
}

export interface ProposalDiff {
  // Keys of the rows that are new, and of the rows (or sections) whose content changed.
  added: string[]
  changed: string[]
  // How many rows were removed.
  removed: number
}

type AnyRow = { id?: string } & Record<string, unknown>

// The content of a row without its id, to tell whether the AI changed it.
function contentOf(item: object): string {
  const rest: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(item)) if (key !== 'id') rest[key] = value
  return JSON.stringify(rest)
}

const LISTS: { kind: RowKind; of: (p: CVImportProposal) => AnyRow[] }[] = [
  { kind: 'skill', of: (p) => p.skills as unknown as AnyRow[] },
  { kind: 'experience', of: (p) => p.experience as unknown as AnyRow[] },
  { kind: 'education', of: (p) => p.education as unknown as AnyRow[] },
]

// mergeProposal carries the review's state over to a newer version of the proposal. A row keeps its state when it keeps its id; a row of a proposal without ids (an older run) keeps it
// when the row at the same position has the same content (the case where the server only gave the rows their ids). A new row starts as the proposal says (ticked when it is to be inserted
// and not a possible duplicate); a removed row's state is dropped; a section keeps its state. The person's own ticks are never changed by the merge.
export function mergeProposal(
  oldP: CVImportProposal,
  newP: CVImportProposal,
  state: ReviewState,
): { state: ReviewState; diff: ProposalDiff } {
  const keyMap = new Map<string, string>([
    ['profile', 'profile'],
    ['persona', 'persona'],
    ['personalDetails', 'personalDetails'],
  ])
  const diff: ProposalDiff = { added: [], changed: [], removed: 0 }
  for (const section of ['profile', 'persona', 'personalDetails'] as const) {
    if (contentOf(oldP[section]) !== contentOf(newP[section])) diff.changed.push(section)
  }
  const has = (key: string) => key in state.checked
  const checked: Record<string, boolean> = {
    profile: has('profile') ? state.checked.profile : isInitiallyChecked(newP.profile),
    persona: has('persona') ? state.checked.persona : isInitiallyChecked(newP.persona),
    personalDetails: has('personalDetails') ? state.checked.personalDetails : isInitiallyChecked(newP.personalDetails),
  }
  for (const { kind, of } of LISTS) {
    const before = of(oldP)
    const after = of(newP)
    const claimed = new Set<number>()
    before.forEach((old, i) => {
      let j = -1
      if (old.id) j = after.findIndex((n) => n.id === old.id)
      else if (after[i] && contentOf(after[i]) === contentOf(old)) j = i
      if (j < 0 || claimed.has(j)) {
        diff.removed += 1
        return
      }
      claimed.add(j)
      const oldKey = rowKey(kind, old, i)
      const newKey = rowKey(kind, after[j], j)
      keyMap.set(oldKey, newKey)
      if (contentOf(after[j]) !== contentOf(old)) diff.changed.push(newKey)
    })
    after.forEach((n, j) => {
      const key = rowKey(kind, n, j)
      if (!claimed.has(j)) {
        diff.added.push(key)
        checked[key] = isInitiallyChecked(n as unknown as { action: 'insert' | 'skip'; possibleDuplicateOf: string | null })
      }
    })
  }
  const carry = <T>(from: Record<string, T>): Record<string, T> => {
    const out: Record<string, T> = {}
    for (const [key, value] of Object.entries(from)) {
      const next = keyMap.get(key)
      if (next !== undefined) out[next] = value
    }
    return out
  }
  const carriedChecked = carry(state.checked)
  return {
    state: {
      checked: { ...checked, ...carriedChecked },
      insertedKeys: carry(state.insertedKeys),
      insertErrors: carry(state.insertErrors),
      insertNotes: carry(state.insertNotes),
    },
    diff,
  }
}

// The line the review shows when the AI changed the suggestions: what changed, in words. Empty when nothing the person can see changed (the server only gave rows their ids).
export function describeDiff(diff: ProposalDiff): string {
  const parts: string[] = []
  if (diff.added.length > 0) parts.push(`${diff.added.length} added`)
  if (diff.changed.length > 0) parts.push(`${diff.changed.length} changed`)
  if (diff.removed > 0) parts.push(`${diff.removed} removed`)
  return parts.length === 0 ? '' : `The AI changed the suggestions in the chat (${parts.join(', ')}).`
}

// What the review's Insert button should do (plan/ai/tools/career/step-167-hide-insert-when-saved.md, step 1). A row is "left to insert"
// when it is ticked and not yet inserted. The button is there while something is left (or an insert is running), gone once everything
// ticked is saved ('saved', a line takes its place), and disabled when nothing was ticked and nothing saved yet.
export type InsertView = 'insert' | 'disabled' | 'saved'

export interface InsertState {
  // The keys of the rows that are ticked and not inserted yet (a failed row stays here: it is what "Insert again" retries).
  remaining: string[]
  // How many rows are inserted.
  insertedCount: number
  // How many of the remaining rows failed their last attempt, so the button says "Insert again".
  failedCount: number
  view: InsertView
}

export function insertState(
  checked: Record<string, boolean>,
  insertedKeys: Record<string, boolean>,
  insertErrors: Record<string, string>,
  inserting: boolean,
): InsertState {
  const remaining = Object.keys(checked).filter((key) => checked[key] && !insertedKeys[key])
  const insertedCount = Object.values(insertedKeys).filter(Boolean).length
  const failedCount = remaining.filter((key) => (insertErrors[key] ?? '') !== '').length
  let view: InsertView
  if (inserting || remaining.length > 0) view = 'insert'
  else if (insertedCount > 0) view = 'saved'
  else view = 'disabled'
  return { remaining, insertedCount, failedCount, view }
}
