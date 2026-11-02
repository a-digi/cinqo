// Pure rules of the import review (moved out of ImportCvPage in plan 166, step 3): the checklist's first state and the save
// summary that is stored on the run after every insert. No React, so they can be script-checked.
import { isInitiallyChecked, type CVImportProposal } from './parseProposal'
import type { InsertResult, ItemResult } from './insertProposal'
import { experienceLabel } from './experienceLabel'

// buildFullSaveSummary reconstructs the InsertResult shape
// cv_import_runs.go's own save_summary_json stores, from this
// component's own accumulated insertedKeys/insertErrors state — not
// from insertSelected's own per-call return value, which only ever
// covers items attempted in THAT call. Takes the two maps as explicit
// params (not read from component state) so a caller can pass the
// just-computed next state before it's actually committed via
// setState — React state updates aren't synchronously readable
// immediately after being requested.
export function buildFullSaveSummary(
  p: CVImportProposal,
  insertedKeys: Record<string, boolean>,
  insertErrors: Record<string, string>,
): InsertResult {
  function itemResult(key: string): ItemResult | undefined {
    if (insertedKeys[key]) return { ok: true }
    if (insertErrors[key]) return { ok: false, error: insertErrors[key] }
    return undefined
  }
  const skills: (ItemResult & { index: number; value: string })[] = []
  p.skills.forEach((s, i) => {
    const r = itemResult(`skill-${i}`)
    if (r) skills.push({ ...r, index: i, value: s.value })
  })
  const experience: (ItemResult & { index: number; label: string })[] = []
  p.experience.forEach((e, i) => {
    const r = itemResult(`experience-${i}`)
    if (r) experience.push({ ...r, index: i, label: experienceLabel(e) })
  })
  return {
    profile: itemResult('profile'),
    persona: itemResult('persona'),
    personalDetails: itemResult('personalDetails'),
    skills,
    experience,
  }
}

export function buildInitialChecked(p: CVImportProposal): Record<string, boolean> {
  const checked: Record<string, boolean> = {
    profile: isInitiallyChecked(p.profile),
    persona: isInitiallyChecked(p.persona),
    personalDetails: isInitiallyChecked(p.personalDetails),
  }
  p.skills.forEach((s, i) => {
    checked[`skill-${i}`] = isInitiallyChecked(s)
  })
  p.experience.forEach((e, i) => {
    checked[`experience-${i}`] = isInitiallyChecked(e)
  })
  return checked
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
