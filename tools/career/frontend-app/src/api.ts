// api.ts — thin fetch helpers over this tool's own proxy routes
// (http.go, step 5), the human-facing mirror of steps 3/4/8's own MCP
// tools. Same PROXY_BASE convention browser's own api.ts already
// established: /api/v1/tools/{slug}/proxy/{path_suffix}.
//
// Step 8 made every profile/skill/experience call require a
// personaId (backend-enforced — an unknown one is a real 400, not a
// silent empty result); step 9 threads that through every function
// here. See plan/ai/tools/career/step-08-persona.md and
// plan/ai/tools/career/step-09-persona-frontend.md.
const PROXY_BASE = '/api/v1/tools/career/proxy'

export interface Persona {
  id: string
  name: string
  description?: string
  createdAt: string
  updatedAt?: string
}

export interface CareerProfile {
  personaId: string
  fullName: string
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
}

export interface CareerExperience {
  id: string
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

export interface ProfileResult {
  profile: CareerProfile | null
  skills: string[]
  experience: CareerExperience[]
}

export interface Job {
  id: string
  sourceUrl: string
  title: string
  company: string
  location: string
  description: string
  postedAt: string
  crawledAt: string
}

export interface JobsResult {
  jobs: Job[]
  total: number
}

async function jsonOrThrow<T>(res: Response, action: string): Promise<T> {
  if (!res.ok) throw new Error(`failed to ${action} (${res.status})`)
  return res.json() as Promise<T>
}

export async function fetchPersonas(): Promise<Persona[]> {
  const res = await fetch(`${PROXY_BASE}/personas`, { credentials: 'include' })
  const data = await jsonOrThrow<{ personas: Persona[] }>(res, 'load personas')
  return data.personas
}

export async function createPersona(name: string, description?: string): Promise<Persona[]> {
  const res = await fetch(`${PROXY_BASE}/personas`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ name, description }),
  })
  const data = await jsonOrThrow<{ personas: Persona[] }>(res, 'create persona')
  return data.personas
}

export async function updatePersona(id: string, args: { name?: string; description?: string }): Promise<Persona[]> {
  const res = await fetch(`${PROXY_BASE}/personas`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...args }),
  })
  const data = await jsonOrThrow<{ personas: Persona[] }>(res, 'update persona')
  return data.personas
}

export async function deletePersona(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/personas?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete persona (${res.status})`)
}

export async function fetchProfile(personaId: string): Promise<ProfileResult> {
  const res = await fetch(`${PROXY_BASE}/profile?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  return jsonOrThrow<ProfileResult>(res, 'load profile')
}

export interface UpdateProfileArgs {
  fullName?: string
  headline?: string
  summary?: string
  location?: string
  desiredTitles?: string
  desiredLocations?: string
  minSalary?: number
}

export async function updateProfile(personaId: string, args: UpdateProfileArgs): Promise<ProfileResult> {
  const res = await fetch(`${PROXY_BASE}/profile`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...args }),
  })
  return jsonOrThrow<ProfileResult>(res, 'update profile')
}

export async function fetchSkills(personaId: string): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/skills?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ skills: string[] }>(res, 'load skills')
  return data.skills
}

export async function addSkill(personaId: string, skill: string): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/skills`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ personaId, skill }),
  })
  const data = await jsonOrThrow<{ skills: string[] }>(res, 'add skill')
  return data.skills
}

export async function removeSkill(personaId: string, skill: string): Promise<void> {
  const res = await fetch(
    `${PROXY_BASE}/skills?personaId=${encodeURIComponent(personaId)}&skill=${encodeURIComponent(skill)}`,
    { method: 'DELETE', credentials: 'include' },
  )
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove skill (${res.status})`)
}

export async function fetchExperience(personaId: string): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ experience: CareerExperience[] }>(res, 'load experience')
  return data.experience
}

export async function addExperience(personaId: string, entry: Omit<CareerExperience, 'id'>): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...entry }),
  })
  const data = await jsonOrThrow<{ experience: CareerExperience[] }>(res, 'add experience')
  return data.experience
}

export type UpdateExperienceArgs = Partial<Omit<CareerExperience, 'id'>> & { id: string }

export async function updateExperience(personaId: string, args: UpdateExperienceArgs): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...args }),
  })
  const data = await jsonOrThrow<{ experience: CareerExperience[] }>(res, 'update experience')
  return data.experience
}

export async function removeExperience(personaId: string, id: string): Promise<void> {
  const res = await fetch(
    `${PROXY_BASE}/experience?personaId=${encodeURIComponent(personaId)}&id=${encodeURIComponent(id)}`,
    { method: 'DELETE', credentials: 'include' },
  )
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove experience (${res.status})`)
}

export async function fetchJobs(query: string, location: string): Promise<JobsResult> {
  const params = new URLSearchParams()
  if (query) params.set('query', query)
  if (location) params.set('location', location)
  const qs = params.toString()
  const res = await fetch(`${PROXY_BASE}/jobs${qs ? `?${qs}` : ''}`, { credentials: 'include' })
  return jsonOrThrow<JobsResult>(res, 'load jobs')
}

export async function removeJob(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove job (${res.status})`)
}
