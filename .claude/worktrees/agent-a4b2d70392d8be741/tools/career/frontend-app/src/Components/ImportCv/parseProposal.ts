// parseProposal.ts — turns the AI's raw final text (step 5's own
// strict JSON contract) into typed state. Defensive by design: a model
// occasionally wraps its JSON in a code fence or adds a stray sentence
// despite being told not to, so this extracts the first {...} block
// rather than assuming the whole string is bare JSON, and validates
// every required field before returning — a malformed/missing field
// throws, letting the caller show the `error` page state instead of
// rendering a checklist built on undefined data. See
// plan/ai/tools/career/import-cv/step-06-proposal-parsing-and-review-ui.md.
export interface ProposalItem {
  action: 'insert' | 'skip'
  possibleDuplicateOf: string | null
}

export interface ProfileProposal extends ProposalItem {
  name: string
}

export interface PersonaProposal extends ProposalItem {
  name: string
  title: string
}

export interface PersonalDetailsProposal extends ProposalItem {
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
}

export interface SkillProposal extends ProposalItem {
  value: string
}

export interface ExperienceProposal extends ProposalItem {
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

export interface CVImportProposal {
  profile: ProfileProposal
  persona: PersonaProposal
  personalDetails: PersonalDetailsProposal
  skills: SkillProposal[]
  experience: ExperienceProposal[]
}

export type ProposalRowTone = 'gray' | 'amber' | 'red'

// possibleDuplicateOf wins over action for color/tone — an item marked
// action:"insert" but also flagged as a possible duplicate still
// renders red (and starts unchecked, see isInitiallyChecked below),
// since a duplicate hit is a signal to slow down and let the human
// decide, not something the AI's own "insert" judgment should be able
// to override visually.
export function proposalRowTone(item: ProposalItem): ProposalRowTone {
  if (item.possibleDuplicateOf !== null) return 'red'
  return item.action === 'insert' ? 'gray' : 'amber'
}

// isInitiallyChecked — a checked row = AI recommends inserting it; an
// unchecked, possible-duplicate row stays unchecked regardless of the
// AI's own action value (a duplicate hit is a signal to slow down and
// let the human decide, not something action:"insert" should be able
// to override into auto-checked). Computed once when a proposal is
// first parsed — the page's own checked state is then fully
// user-controlled and never re-derived from this again.
export function isInitiallyChecked(item: ProposalItem): boolean {
  return item.possibleDuplicateOf === null && item.action === 'insert'
}

function extractJSONObject(text: string): string {
  const start = text.indexOf('{')
  const end = text.lastIndexOf('}')
  if (start === -1 || end === -1 || end < start) {
    throw new Error("the AI's reply did not contain a JSON object")
  }
  return text.slice(start, end + 1)
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isAction(v: unknown): v is 'insert' | 'skip' {
  return v === 'insert' || v === 'skip'
}

// Lenient on purpose: a model omitting an empty field (null/undefined)
// is treated as "" rather than a parse failure — only a genuinely
// wrong type (a number where a string was expected, etc.) throws.
function asString(v: unknown, field: string): string {
  if (v === null || v === undefined) return ''
  if (typeof v !== 'string') throw new Error(`expected "${field}" to be a string`)
  return v
}

function asNullableString(v: unknown, field: string): string | null {
  if (v === null || v === undefined) return null
  if (typeof v !== 'string') throw new Error(`expected "${field}" to be a string or null`)
  return v
}

function asNumber(v: unknown, field: string): number {
  if (v === null || v === undefined) return 0
  if (typeof v !== 'number') throw new Error(`expected "${field}" to be a number`)
  return v
}

function parseProposalItem(raw: unknown, path: string): ProposalItem {
  if (!isRecord(raw)) throw new Error(`expected "${path}" to be an object`)
  if (!isAction(raw.action)) throw new Error(`expected "${path}.action" to be "insert" or "skip"`)
  return { action: raw.action, possibleDuplicateOf: asNullableString(raw.possibleDuplicateOf, `${path}.possibleDuplicateOf`) }
}

export function parseProposal(rawText: string): CVImportProposal {
  let root: unknown
  try {
    root = JSON.parse(extractJSONObject(rawText)) as unknown
  } catch (err) {
    if (err instanceof Error && err.message.includes('did not contain a JSON object')) throw err
    throw new Error("the AI's reply was not valid JSON")
  }
  if (!isRecord(root)) throw new Error("the AI's reply JSON was not an object")

  const profileRaw = root.profile
  if (!isRecord(profileRaw)) throw new Error('expected "profile" to be an object')
  const profile: ProfileProposal = { ...parseProposalItem(profileRaw, 'profile'), name: asString(profileRaw.name, 'profile.name') }

  const personaRaw = root.persona
  if (!isRecord(personaRaw)) throw new Error('expected "persona" to be an object')
  const persona: PersonaProposal = {
    ...parseProposalItem(personaRaw, 'persona'),
    name: asString(personaRaw.name, 'persona.name'),
    title: asString(personaRaw.title, 'persona.title'),
  }

  const detailsRaw = root.personalDetails
  if (!isRecord(detailsRaw)) throw new Error('expected "personalDetails" to be an object')
  const personalDetails: PersonalDetailsProposal = {
    ...parseProposalItem(detailsRaw, 'personalDetails'),
    headline: asString(detailsRaw.headline, 'personalDetails.headline'),
    summary: asString(detailsRaw.summary, 'personalDetails.summary'),
    location: asString(detailsRaw.location, 'personalDetails.location'),
    desiredTitles: asString(detailsRaw.desiredTitles, 'personalDetails.desiredTitles'),
    desiredLocations: asString(detailsRaw.desiredLocations, 'personalDetails.desiredLocations'),
    minSalary: asNumber(detailsRaw.minSalary, 'personalDetails.minSalary'),
  }

  if (!Array.isArray(root.skills)) throw new Error('expected "skills" to be an array')
  const skills: SkillProposal[] = root.skills.map((raw: unknown, i) => ({
    ...parseProposalItem(raw, `skills[${i}]`),
    value: asString(isRecord(raw) ? raw.value : undefined, `skills[${i}].value`),
  }))

  if (!Array.isArray(root.experience)) throw new Error('expected "experience" to be an array')
  const experience: ExperienceProposal[] = root.experience.map((raw: unknown, i) => {
    const path = `experience[${i}]`
    const obj = isRecord(raw) ? raw : {}
    return {
      ...parseProposalItem(raw, path),
      company: asString(obj.company, `${path}.company`),
      title: asString(obj.title, `${path}.title`),
      startDate: asString(obj.startDate, `${path}.startDate`),
      endDate: asString(obj.endDate, `${path}.endDate`),
      description: asString(obj.description, `${path}.description`),
    }
  })

  return { profile, persona, personalDetails, skills, experience }
}
