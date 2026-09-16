// api.ts — thin fetch helpers over this tool's own proxy routes
// (http.go, step 5), the human-facing mirror of steps 3/4/8/10's own
// MCP tools. Same PROXY_BASE convention browser's own api.ts already
// established: /api/v1/tools/{slug}/proxy/{path_suffix}.
//
// Step 8 made every persona-scoped call require a personaId
// (backend-enforced — an unknown one is a real 400, not a silent
// empty result); step 9 threaded that through every function here.
// Step 10 introduced Profile (the job seeker) one level above
// Persona, and renamed what used to be called "profile" (headline/
// summary/location/desired titles/min salary) to "persona details" —
// that was never the job seeker's own profile, it was a persona's own
// career positioning. Step 11 threads Profile through this file. See
// plan/ai/tools/career/step-08-persona.md,
// plan/ai/tools/career/step-10-job-seeker-profile.md, and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
const PROXY_BASE = '/api/v1/tools/career/proxy'

export interface ProfileExternalLink {
  platform: string
  url: string
}

export interface Profile {
  id: string
  firstName: string
  lastName: string
  createdAt: string
  updatedAt?: string
  externalLinks: ProfileExternalLink[]
}

export interface Persona {
  id: string
  profileId: string
  name: string
  description?: string
  createdAt: string
  updatedAt?: string
}

export interface PersonaDetails {
  personaId: string
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

export interface PersonaDetailsResult {
  personaDetails: PersonaDetails | null
  skills: string[]
  experience: CareerExperience[]
}

export interface Job {
  id: string
  sourceUrl: string
  title: string
  company: string
  companyId?: string
  portalId?: string
  portalName?: string
  location: string
  description: string
  postedAt: string
  crawledAt: string
}

export interface JobsResult {
  jobs: Job[]
  total: number
}

export interface Company {
  id: string
  name: string
  description: string
  jobCount: number
  recruiterCount: number
  createdAt: string
  updatedAt?: string
}

export interface Recruiter {
  id: string
  companyId: string
  firstName: string
  lastName: string
  email: string
  createdAt: string
  updatedAt?: string
}

export interface PortalLink {
  id: string
  portalId: string
  url: string
  title: string | null
  crawlInstructions: string | null
  // Most recent job crawled_at against this link (step 26) — null
  // when no job has ever been saved for it. Read-only, derived.
  lastCrawledAt: string | null
  // Most recent failure of an AI-driven crawl-instructions generation/
  // edit attempt for this link (step 33) — both null when no failure
  // is currently recorded. Read-only; written only via
  // updatePortalLink's own instructionsAiError field.
  instructionsAiError: string | null
  instructionsAiErrorAt: string | null
  // The hidden conversation currently generating/updating this link's
  // own crawl instructions, if one is in flight (step 60) — null once
  // it finishes (success or failure). Read-only; written only via
  // updatePortalLink's own instructionsAiConversationId field. See
  // plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
  instructionsAiConversationId: string | null
  // True while a detached "Crawl now" run is in progress for this link
  // (step 37) — read-only, derived from crawl_runs server-side. Lets
  // the frontend resume watching a run still going after a page
  // reload/reopen. See
  // plan/ai/tools/career/step-38-crawl-now-polling-frontend.md.
  hasActiveCrawlRun: boolean
  createdAt: string
  updatedAt?: string
}

export interface Portal {
  id: string
  name: string
  links: PortalLink[]
  createdAt: string
  updatedAt?: string
}

async function jsonOrThrow<T>(res: Response, action: string): Promise<T> {
  if (!res.ok) throw new Error(`failed to ${action} (${res.status})`)
  return res.json() as Promise<T>
}

// --- profiles (job seekers) ---

export async function fetchProfiles(): Promise<Profile[]> {
  const res = await fetch(`${PROXY_BASE}/profiles`, { credentials: 'include' })
  const data = await jsonOrThrow<{ profiles: Profile[] }>(res, 'load profiles')
  return data.profiles
}

export async function createProfile(firstName: string, lastName: string): Promise<Profile[]> {
  const res = await fetch(`${PROXY_BASE}/profiles`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ firstName, lastName }),
  })
  const data = await jsonOrThrow<{ profiles: Profile[] }>(res, 'create profile')
  return data.profiles
}

export async function updateProfile(id: string, args: { firstName?: string; lastName?: string }): Promise<Profile[]> {
  const res = await fetch(`${PROXY_BASE}/profiles`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...args }),
  })
  const data = await jsonOrThrow<{ profiles: Profile[] }>(res, 'update profile')
  return data.profiles
}

export async function deleteProfile(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profiles?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete profile (${res.status})`)
}

export async function addProfileExternalLink(profileId: string, platform: string, url: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profile-links`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, platform, url }),
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to add external link (${res.status})`)
}

export async function removeProfileExternalLink(profileId: string, platform: string): Promise<void> {
  const res = await fetch(
    `${PROXY_BASE}/profile-links?profileId=${encodeURIComponent(profileId)}&platform=${encodeURIComponent(platform)}`,
    { method: 'DELETE', credentials: 'include' },
  )
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove external link (${res.status})`)
}

// --- personas ---

export async function fetchPersonas(profileId?: string): Promise<Persona[]> {
  const qs = profileId ? `?profileId=${encodeURIComponent(profileId)}` : ''
  const res = await fetch(`${PROXY_BASE}/personas${qs}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ personas: Persona[] }>(res, 'load personas')
  return data.personas
}

export async function createPersona(profileId: string, name: string, description?: string): Promise<Persona[]> {
  const res = await fetch(`${PROXY_BASE}/personas`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, name, description }),
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

// --- persona details ---

export async function fetchPersonaDetails(personaId: string): Promise<PersonaDetailsResult> {
  const res = await fetch(`${PROXY_BASE}/persona-details?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  return jsonOrThrow<PersonaDetailsResult>(res, 'load persona details')
}

export interface UpdatePersonaDetailsArgs {
  headline?: string
  summary?: string
  location?: string
  desiredTitles?: string
  desiredLocations?: string
  minSalary?: number
}

export async function updatePersonaDetails(personaId: string, args: UpdatePersonaDetailsArgs): Promise<PersonaDetailsResult> {
  const res = await fetch(`${PROXY_BASE}/persona-details`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...args }),
  })
  return jsonOrThrow<PersonaDetailsResult>(res, 'update persona details')
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
  const res = await fetch(`${PROXY_BASE}/skills?personaId=${encodeURIComponent(personaId)}&skill=${encodeURIComponent(skill)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
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
  const res = await fetch(`${PROXY_BASE}/experience?personaId=${encodeURIComponent(personaId)}&id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove experience (${res.status})`)
}

export async function fetchJobs(
  query: string,
  location: string,
  companyId?: string,
  portalId?: string,
  limit?: number,
  offset?: number,
): Promise<JobsResult> {
  const params = new URLSearchParams()
  if (query) params.set('query', query)
  if (location) params.set('location', location)
  if (companyId) params.set('companyId', companyId)
  if (portalId) params.set('portalId', portalId)
  if (limit !== undefined) params.set('limit', String(limit))
  if (offset !== undefined) params.set('offset', String(offset))
  const qs = params.toString()
  const res = await fetch(`${PROXY_BASE}/jobs${qs ? `?${qs}` : ''}`, { credentials: 'include' })
  return jsonOrThrow<JobsResult>(res, 'load jobs')
}

export async function fetchJobLocations(): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/job-locations`, { credentials: 'include' })
  const data = await jsonOrThrow<{ locations: string[] }>(res, 'load job locations')
  return data.locations
}

export async function removeJob(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove job (${res.status})`)
}

export async function linkJobToCompany(jobId: string, companyId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id: jobId, companyId }),
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to link job to company (${res.status})`)
}

// --- companies ---

export async function fetchCompanies(): Promise<Company[]> {
  const res = await fetch(`${PROXY_BASE}/companies`, { credentials: 'include' })
  const data = await jsonOrThrow<{ companies: Company[] }>(res, 'load companies')
  return data.companies
}

export async function createCompany(name: string, description: string): Promise<Company[]> {
  const res = await fetch(`${PROXY_BASE}/companies`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ name, description }),
  })
  const data = await jsonOrThrow<{ companies: Company[] }>(res, 'create company')
  return data.companies
}

export async function updateCompany(id: string, args: { name?: string; description?: string }): Promise<Company[]> {
  const res = await fetch(`${PROXY_BASE}/companies`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...args }),
  })
  const data = await jsonOrThrow<{ companies: Company[] }>(res, 'update company')
  return data.companies
}

export async function removeCompany(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/companies?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete company (${res.status})`)
}

// --- recruiters ---

export async function fetchRecruiters(companyId?: string): Promise<Recruiter[]> {
  const qs = companyId ? `?companyId=${encodeURIComponent(companyId)}` : ''
  const res = await fetch(`${PROXY_BASE}/recruiters${qs}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ recruiters: Recruiter[] }>(res, 'load recruiters')
  return data.recruiters
}

export async function createRecruiter(companyId: string, firstName: string, lastName: string, email: string): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/recruiters`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ companyId, firstName, lastName, email }),
  })
  return jsonOrThrow<{ id: string }>(res, 'create recruiter')
}

export async function updateRecruiter(id: string, args: { firstName?: string; lastName?: string; email?: string }): Promise<Recruiter[]> {
  const res = await fetch(`${PROXY_BASE}/recruiters`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...args }),
  })
  const data = await jsonOrThrow<{ recruiters: Recruiter[] }>(res, 'update recruiter')
  return data.recruiters
}

export async function removeRecruiter(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/recruiters?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete recruiter (${res.status})`)
}

// --- portals ---

export async function fetchPortals(): Promise<Portal[]> {
  const res = await fetch(`${PROXY_BASE}/portals`, { credentials: 'include' })
  const data = await jsonOrThrow<{ portals: Portal[] }>(res, 'load portals')
  return data.portals
}

export async function createPortal(name: string): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/portals`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ name }),
  })
  return jsonOrThrow<{ id: string }>(res, 'create portal')
}

export async function updatePortal(id: string, name: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portals`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, name }),
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to update portal (${res.status})`)
}

export async function removePortal(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portals?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete portal (${res.status})`)
}

export async function addPortalLink(portalId: string, url: string, title: string): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/portal-links`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalId, url, title }),
  })
  return jsonOrThrow<{ id: string }>(res, 'add portal link')
}

export async function updatePortalLink(
  id: string,
  args: { url?: string; title?: string; crawlInstructions?: string; instructionsAiError?: string; instructionsAiConversationId?: string },
): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portal-links`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...args }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `failed to update portal link (${res.status})`)
  }
}

export async function removePortalLink(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portal-links?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove portal link (${res.status})`)
}
