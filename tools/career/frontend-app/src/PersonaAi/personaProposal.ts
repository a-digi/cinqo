// The proposal an AI run ends with (step 112): parsing the AI's final reply and validating it.
// The limits mirror the server's (backend persona/ai_proposal.go), which validates again before
// anything is stored or created. Pure, so it can be script-checked.

import type { PersonaProposal, ProposedPersona, ProposedRole } from '../api'

export const MAX_PERSONAS = 3
export const MAX_SHORT = 300
export const MAX_LONG = 4000
export const MAX_SKILLS = 60
export const MAX_ROLES = 30
export const MAX_NOTE = 1000

export type ParseResult = { ok: true; proposal: PersonaProposal } | { ok: false; error: string }

const hasMarkup = (s: string) => /<[/!a-zA-Z]/.test(s)

function str(value: unknown, field: string, max: number, required = false): string {
  if (value === undefined || value === null) {
    if (required) throw new Error(`${field} is missing`)
    return ''
  }
  if (typeof value !== 'string') throw new Error(`${field} must be text`)
  const s = value.trim()
  if (required && s === '') throw new Error(`${field} is empty`)
  if (s.length > max) throw new Error(`${field} is too long (max ${max} characters)`)
  if (hasMarkup(s)) throw new Error(`${field} must be plain text, no markup`)
  return s
}

function persona(raw: unknown, n: number): ProposedPersona {
  const at = `persona ${n}`
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) throw new Error(`${at} is not an object`)
  const r = raw as Record<string, unknown>
  const d = typeof r.details === 'object' && r.details !== null ? (r.details as Record<string, unknown>) : {}
  const salary = d.minSalary ?? 0
  if (typeof salary !== 'number' || !Number.isFinite(salary) || salary < 0) throw new Error(`${at}: minSalary must be a number, 0 or more`)

  const skillsRaw = r.skills ?? []
  if (!Array.isArray(skillsRaw)) throw new Error(`${at}: skills must be a list`)
  if (skillsRaw.length > MAX_SKILLS) throw new Error(`${at}: too many skills (max ${MAX_SKILLS})`)
  const seen = new Set<string>()
  const skills: string[] = []
  for (const s of skillsRaw) {
    const skill = str(s, `${at}: a skill`, MAX_SHORT, true)
    if (!seen.has(skill.toLowerCase())) {
      seen.add(skill.toLowerCase())
      skills.push(skill)
    }
  }

  const rolesRaw = r.experience ?? []
  if (!Array.isArray(rolesRaw)) throw new Error(`${at}: experience must be a list`)
  if (rolesRaw.length > MAX_ROLES) throw new Error(`${at}: too many experience entries (max ${MAX_ROLES})`)
  const experience: ProposedRole[] = rolesRaw.map((e: unknown, i) => {
    if (typeof e !== 'object' || e === null) throw new Error(`${at}: experience ${i + 1} is not an object`)
    const x = e as Record<string, unknown>
    return {
      company: str(x.company, `${at}: experience ${i + 1} company`, MAX_SHORT, true),
      title: str(x.title, `${at}: experience ${i + 1} title`, MAX_SHORT, true),
      startDate: str(x.startDate, `${at}: experience ${i + 1} startDate`, MAX_SHORT),
      endDate: str(x.endDate, `${at}: experience ${i + 1} endDate`, MAX_SHORT),
      description: str(x.description, `${at}: experience ${i + 1} description`, MAX_LONG),
    }
  })

  const dup = r.possibleDuplicateOf
  return {
    name: str(r.name, `${at}: name`, MAX_SHORT, true),
    description: str(r.description, `${at}: description`, MAX_SHORT),
    possibleDuplicateOf: typeof dup === 'string' && dup.trim() !== '' ? dup.trim() : null,
    details: {
      headline: str(d.headline, `${at}: headline`, MAX_SHORT),
      summary: str(d.summary, `${at}: summary`, MAX_LONG),
      location: str(d.location, `${at}: location`, MAX_SHORT),
      desiredTitles: str(d.desiredTitles, `${at}: desiredTitles`, MAX_SHORT),
      desiredLocations: str(d.desiredLocations, `${at}: desiredLocations`, MAX_SHORT),
      minSalary: Math.round(salary),
    },
    skills,
    experience,
  }
}

// The JSON text inside the AI's reply: the whole reply, a fenced block, or the outermost braces.
export function extractJson(reply: string): string | null {
  const text = reply.trim()
  const fenced = /^```(?:json)?\s*([\s\S]*?)\s*```$/i.exec(text)
  if (fenced) return fenced[1]
  if (text.startsWith('{')) return text
  const first = text.indexOf('{')
  const last = text.lastIndexOf('}')
  return first >= 0 && last > first ? text.slice(first, last + 1) : null
}

// Parses the AI's final reply into a validated proposal, or says what is wrong with it.
export function parseProposalReply(reply: string): ParseResult {
  const json = extractJson(reply)
  if (json === null) return { ok: false, error: 'the reply has no JSON in it' }
  let raw: unknown
  try {
    raw = JSON.parse(json)
  } catch {
    return { ok: false, error: 'the reply is not valid JSON' }
  }
  try {
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) throw new Error('the reply is not a JSON object')
    const r = raw as Record<string, unknown>
    if (!Array.isArray(r.personas)) throw new Error('personas must be a list (an empty list when there is nothing to propose)')
    if (r.personas.length > MAX_PERSONAS) throw new Error(`too many personas (max ${MAX_PERSONAS})`)
    const personas = r.personas.map((p: unknown, i) => persona(p, i + 1))
    const names = new Set<string>()
    for (const p of personas) {
      const key = p.name.toLowerCase()
      if (names.has(key)) throw new Error(`the name "${p.name}" is used twice`)
      names.add(key)
    }
    return { ok: true, proposal: { personas, note: str(r.note, 'note', MAX_NOTE) } }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : 'the proposal is invalid' }
  }
}

// What is wrong with the personas the user accepted (possibly edited), or null when they can be
// created: the same checks the AI's reply gets, and unique names among them.
export function checkAccepted(personas: ProposedPersona[]): string | null {
  if (personas.length === 0) return 'Pick at least one persona to create.'
  if (personas.length > MAX_PERSONAS) return `Too many personas (max ${MAX_PERSONAS}).`
  const names = new Set<string>()
  try {
    personas.forEach((p, i) => {
      const checked = persona(p, i + 1)
      const key = checked.name.toLowerCase()
      if (names.has(key)) throw new Error(`the name "${checked.name}" is used twice`)
      names.add(key)
    })
  } catch (err) {
    return err instanceof Error ? err.message : 'A persona is invalid.'
  }
  return null
}
