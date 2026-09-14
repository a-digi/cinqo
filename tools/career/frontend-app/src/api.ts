// api.ts — thin fetch helpers over this tool's own proxy routes
// (http.go, step 5), the human-facing mirror of steps 3/4's own MCP
// tools. Same PROXY_BASE convention browser's own api.ts already
// established: /api/v1/tools/{slug}/proxy/{path_suffix}.
const PROXY_BASE = '/api/v1/tools/career/proxy'

export interface CareerProfile {
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

export async function fetchProfile(): Promise<ProfileResult> {
  const res = await fetch(`${PROXY_BASE}/profile`, { credentials: 'include' })
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

export async function updateProfile(args: UpdateProfileArgs): Promise<ProfileResult> {
  const res = await fetch(`${PROXY_BASE}/profile`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(args),
  })
  return jsonOrThrow<ProfileResult>(res, 'update profile')
}

export async function addSkill(skill: string): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/skills`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ skill }),
  })
  const data = await jsonOrThrow<{ skills: string[] }>(res, 'add skill')
  return data.skills
}

export async function removeSkill(skill: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/skills?skill=${encodeURIComponent(skill)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove skill (${res.status})`)
}

export async function addExperience(entry: Omit<CareerExperience, 'id'>): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(entry),
  })
  const data = await jsonOrThrow<{ experience: CareerExperience[] }>(res, 'add experience')
  return data.experience
}

export async function removeExperience(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/experience?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
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
