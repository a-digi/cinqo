// parseProposal.ts — turns the proposal the AI handed over (stored on the import run by the suggest_cv_import tool, step 155) into
// typed state. The server already validated it; this is the page's own second check, so every required field is verified
// again and a malformed or missing one throws, letting the caller show the `error` page state instead of rendering a
// checklist built on undefined data. See plan/ai/tools/career/import-cv/step-06-proposal-parsing-and-review-ui.md and
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
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

// The list items carry an id of their own once the server gave them one (plan 257); a proposal stored before that has none, and the page then keys its rows by position.
export interface SkillProposal extends ProposalItem {
  id?: string
  value: string
}

export interface ExperienceProposal extends ProposalItem {
  id?: string
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

// One school, degree or course found in the CV (plan 175).
export interface EducationProposal extends ProposalItem {
  id?: string
  institution: string
  degree: string
  fieldOfStudy: string
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
  // Empty for a proposal stored before education existed or for a CV without any.
  education: EducationProposal[]
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

// The id of a list item: a text the server made, or none (an older proposal).
function asId(raw: unknown): { id?: string } {
  return isRecord(raw) && typeof raw.id === 'string' && raw.id !== '' ? { id: raw.id } : {}
}

function parseProposalItem(raw: unknown, path: string): ProposalItem {
  if (!isRecord(raw)) throw new Error(`expected "${path}" to be an object`)
  if (!isAction(raw.action)) throw new Error(`expected "${path}.action" to be "insert" or "skip"`)
  return { action: raw.action, possibleDuplicateOf: asNullableString(raw.possibleDuplicateOf, `${path}.possibleDuplicateOf`) }
}

// The proposal the AI handed over with suggest_cv_import, as the server stored it (step 155): a value, not text to dig JSON out
// of. The server validates it before storing it; this is the page's own second check, so a stored proposal of an unexpected
// shape is an error here and never a checklist built on undefined data.
export function parseProposal(root: unknown): CVImportProposal {
  if (!isRecord(root)) throw new Error('the stored proposal was not an object')

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
    ...asId(raw),
    ...parseProposalItem(raw, `skills[${i}]`),
    value: asString(isRecord(raw) ? raw.value : undefined, `skills[${i}].value`),
  }))

  if (!Array.isArray(root.experience)) throw new Error('expected "experience" to be an array')
  const experience: ExperienceProposal[] = root.experience.map((raw: unknown, i) => {
    const path = `experience[${i}]`
    const obj = isRecord(raw) ? raw : {}
    return {
      ...asId(raw),
      ...parseProposalItem(raw, path),
      company: asString(obj.company, `${path}.company`),
      title: asString(obj.title, `${path}.title`),
      startDate: asString(obj.startDate, `${path}.startDate`),
      endDate: asString(obj.endDate, `${path}.endDate`),
      description: asString(obj.description, `${path}.description`),
    }
  })

  // Optional on purpose (plan 175): a missing or null list is none; anything else that is not a list is an error.
  let education: EducationProposal[] = []
  if (root.education !== undefined && root.education !== null) {
    if (!Array.isArray(root.education)) throw new Error('expected "education" to be an array')
    education = root.education.map((raw: unknown, i) => {
      const path = `education[${i}]`
      const obj = isRecord(raw) ? raw : {}
      return {
        ...asId(raw),
        ...parseProposalItem(raw, path),
        institution: asString(obj.institution, `${path}.institution`),
        degree: asString(obj.degree, `${path}.degree`),
        fieldOfStudy: asString(obj.fieldOfStudy, `${path}.fieldOfStudy`),
        startDate: asString(obj.startDate, `${path}.startDate`),
        endDate: asString(obj.endDate, `${path}.endDate`),
        description: asString(obj.description, `${path}.description`),
      }
    })
  }

  return { profile, persona, personalDetails, skills, experience, education }
}
