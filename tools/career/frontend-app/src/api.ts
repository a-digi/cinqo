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

export interface CompletenessItem {
  key: string
  label: string
  weight: number
  done: boolean
}

// How complete a profile is, out of 100, worked out by the backend on every read.
export interface ProfileCompleteness {
  score: number
  band: 'just-started' | 'getting-there' | 'strong' | 'complete'
  items: CompletenessItem[]
}

export interface Profile {
  id: string
  firstName: string
  lastName: string
  createdAt: string
  updatedAt?: string
  externalLinks: ProfileExternalLink[]
  completeness: ProfileCompleteness
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
  // sourceUrl is "" for a job added by hand without a posting URL (step 97).
  sourceUrl: string
  // source (step 97) — "crawled" (a crawl or the AI saved it) or "manual" (a
  // person added it). Absent from a backend that predates it.
  source?: 'crawled' | 'manual'
  title: string
  company: string
  companyId?: string
  portalId?: string
  portalName?: string
  location?: string
  description?: string
  postedAt: string
  crawledAt: string
  // detailCrawlStatus (step XX) — undefined means no job-detail-page
  // crawl attempt has ever been made for this job, as opposed to
  // crawledAt, which is set unconditionally by the listing crawl that
  // produced this row. See plan/ai/tools/career/step-XX-job-detail-
  // crawl-status-eye-icon.md.
  detailCrawlStatus?: 'failed' | 'success'
  // matchScore/matchStatus/matchConversationId/matchError (step XX) —
  // mirror detailCrawlStatus's own "undefined means never attempted"
  // posture, for the separate "Job Match" feature. matchScore is only
  // ever present once matchStatus is 'completed'. See
  // plan/ai/tools/career/step-XX-job-match.md.
  matchScore?: number
  matchStatus?: 'matching' | 'completed' | 'failed'
  matchConversationId?: string
  matchError?: string
  // matchedSkills (step XX) — the specific skills (verbatim, from the
  // matched persona's own skills list) that explain matchScore. Always
  // present as an array (possibly empty), never undefined — the
  // backend guarantees this field, unlike the optional ones above. See
  // plan/ai/tools/career/step-XX-job-match-skills.md.
  matchedSkills: string[]
  // cvStatus/cvConversationId/cvMediaFileId/cvError (step XX) mirror
  // matchStatus's own "undefined means never attempted" posture, for
  // the "Generate CV PDF" feature. The backend's own conversation
  // orchestrator promotes the AI-rendered PDF into permanent Media
  // storage entirely server-side (no frontend involvement) before
  // cvStatus ever reaches "completed" — cvMediaFileId is set at that
  // point; build a download link from it via mediaDownloadUrl(). See
  // plan/ai/tools/career/step-XX-cv-pdf.md.
  cvStatus?: 'generating' | 'completed' | 'failed'
  cvConversationId?: string
  cvMediaFileId?: string
  cvError?: string
  // cvSource/cvDocumentId (step 98) — where the job's current CV came from:
  // the AI ("ai"), a CV built in the CV Builder for this job ("manual"), or one
  // picked from the library ("existing"); cvDocumentId is the cv_documents row
  // when known. Absent when the job has no CV.
  cvSource?: 'ai' | 'manual' | 'existing'
  cvDocumentId?: string
}

export interface JobsResult {
  jobs: Job[]
  total: number
}

// One past CV generation attempt for a job — the full audit trail,
// unlike Job's own cvStatus/cvConversationId/cvMediaFileId/cvError
// (which only ever reflect the CURRENT attempt, wiped to undefined the
// moment a later one starts). See
// plan/ai/career/cv-generation-audit/step-05-jobs-page-ui.md.
export interface CvGeneration {
  id: string
  conversationId: string
  status: 'generating' | 'completed' | 'failed'
  cvDocumentId?: string
  mediaFileId?: string
  error?: string
  startedAt: string
  finishedAt?: string
}

export async function fetchCvGenerations(jobId: string): Promise<CvGeneration[]> {
  const res = await fetch(`${PROXY_BASE}/jobs/cv-generations?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  const body = await jsonOrThrow<{ generations: CvGeneration[] }>(res, 'load cv generations')
  return body.generations
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

export interface JobsQuestion {
  askedAt: string
  note: string
}

export interface PortalLink {
  id: string
  portalId: string
  url: string
  title: string | null
  crawlInstructions: string | null
  // Separate, second instruction document — how to extract job-
  // position-relevant text off a single job's own detail page, rather
  // than crawlInstructions' own listing-page fields+pagination shape.
  // See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
  jobDetailCrawlInstructions: string | null
  // Separate, job-detail-specific counterparts to
  // instructionsAiError/instructionsAiErrorAt/instructionsAiConversationId
  // below, which track the LISTING instructions' own AI generation
  // instead. See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
  jobDetailInstructionsAiError: string | null
  jobDetailInstructionsAiErrorAt: string | null
  jobDetailInstructionsAiConversationId: string | null
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
  // listingCrawlRequested/jobDetailInstructionsAiRequested (step 65-67)
  // are request flags a backend Domain Events listener sets to ask the
  // frontend to perform a privileged action (start a crawl, start an AI
  // conversation) under the current user's own live session — a
  // backend event handler has no live end-user session to act as
  // itself. See
  // plan/ai/tools/career/step-64-event-driven-crawl-instructions.md.
  // CrawlPanel.tsx's own resume-style effects consume these the same
  // way it already resumes an in-flight AI conversation after a
  // reload.
  listingCrawlRequested: boolean
  jobDetailInstructionsAiRequested: boolean
  // instructionsAiErrorDismissedAt/jobDetailInstructionsAiErrorDismissedAt
  // (step 71) record when the user last dismissed that document's own
  // AI-generation error banner — compared against
  // instructionsAiErrorAt/jobDetailInstructionsAiErrorAt at render time:
  // a dismissal at least as recent as the error means "stay hidden," a
  // NEWER error means a fresh failure that should show again. See
  // plan/ai/tools/career/step-71-dismiss-tracking-data-model.md.
  instructionsAiErrorDismissedAt: string | null
  jobDetailInstructionsAiErrorDismissedAt: string | null
  // jobDetailCrawlRequested (step 76) — same request-flag shape as
  // listingCrawlRequested/jobDetailInstructionsAiRequested above, set
  // unconditionally by the same events/jobs-crawled listener the
  // instant a listing crawl finishes. See
  // plan/ai/tools/career/step-76-job-detail-crawl-requested-backend.md.
  jobDetailCrawlRequested: boolean
  // autoDiscoveryEnabled (step 84 follow-up) gates whether this
  // specific link participates in the auto-discovery manager's own
  // sweeps — the global auto-discovery on/off+interval setting
  // (Crawler/autoDiscovery.ts) controls whether the mechanism runs at
  // ALL; this is the per-link override on top of that ("some of the
  // links people might not want to be auto-discovered"). Defaults
  // true (on). See
  // plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
  autoDiscoveryEnabled: boolean
  // autoDiscoveryIntervalMinutes (step 85) is this link's own override
  // on the schedule, checked against its own autoDiscoveryLastRunAt
  // instead of the global setting — absent (the backend's own
  // `*int` + omitempty means the field is OMITTED, not sent as null,
  // when unset) means "use the global interval," every link's own
  // default. Only meaningful while autoDiscoveryEnabled is also true.
  // See plan/ai/tools/career/step-85-auto-discovery-per-link-custom-interval.md.
  autoDiscoveryIntervalMinutes?: number
  // autoDiscoveryLastRunAt (step 85) is this link's own independent
  // unix-seconds clock — read-only, written only by the auto-discovery
  // manager, absent the same way until it's ever run; only ever
  // meaningful while autoDiscoveryIntervalMinutes is set. A link using
  // the global interval keeps relying on that setting's own
  // lastRunAt instead (Crawler/autoDiscovery.ts).
  autoDiscoveryLastRunAt?: number
  // instructionStats (step 86) — this link's "Generate with AI"
  // attempt counts per instruction kind; read-only, always present
  // (all zeros for a link with no attempts).
  instructionStats: InstructionStats
  // lastListingCrawl (step 90) — the latest completed listing crawl and
  // what is known about a zero-job result; absent when there is no
  // structured data (never crawled, or only runs from before step 90).
  lastListingCrawl?: LastListingCrawl
  // jobsQuestion (step 91) — the pending "does this page list jobs?"
  // question the generation agent asked; absent when nothing is pending.
  // note is the agent's own text: show it as plain text only.
  jobsQuestion?: JobsQuestion
  createdAt: string
  updatedAt?: string
}

export interface LastListingCrawl {
  finishedAt: string
  triggeredBy: 'manual' | 'auto_discovery'
  jobsSaved: number
  jobsUpdated: number
  jobsSkipped: number
  pagesVisited: number
  stoppedReason: string
  // Field labels that matched nothing on every visited page. Empty in
  // grouped mode (a container is set), where the browser doesn't report
  // them — empty there does NOT mean everything matched.
  notFoundFields: string[]
  zeroStreak: number
  lastJobsAt?: string
  jobsFromThisLink: number
  acknowledgedAt?: string
}

export type InstructionKind = 'listing' | 'job_detail'

// attempts counts finished runs (succeeded + failed); a run still in
// flight is only reflected in running. See
// plan/ai/tools/career/step-86-instruction-attempt-tracking.md.
export interface InstructionKindStats {
  attempts: number
  succeeded: number
  failed: number
  running: boolean
  lastFailureAt?: string
  lastError?: string
  // consecutiveFailures (step 87) — runs since the last success that
  // ended without the AI saving instructions; provider errors and
  // interruptions don't count and don't reset it.
  consecutiveFailures: number
}

export interface InstructionStats {
  listing: InstructionKindStats
  jobDetail: InstructionKindStats
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

// The backend's own POST response already includes the new row's id
// alongside the refreshed list (http.go's profilesHandler) — surfaced
// here since Import CV's own insert flow (step 7) needs a reliable id
// to chain a persona creation onto, not "guess via array order/name."
export async function createProfile(firstName: string, lastName: string): Promise<{ id: string; profiles: Profile[] }> {
  const res = await fetch(`${PROXY_BASE}/profiles`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ firstName, lastName }),
  })
  return jsonOrThrow<{ id: string; profiles: Profile[] }>(res, 'create profile')
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

// Same reasoning as createProfile above — the backend already returns
// the new persona's own id, surfaced here for the same reason.
export async function createPersona(profileId: string, name: string, description?: string): Promise<{ id: string; personas: Persona[] }> {
  const res = await fetch(`${PROXY_BASE}/personas`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, name, description }),
  })
  return jsonOrThrow<{ id: string; personas: Persona[] }>(res, 'create persona')
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

// The backend leaves out an entry's empty text fields (an entry with no end date comes
// back with no endDate at all), so fill them in: the pages treat all five as strings.
function normalizeExperience(entries: Partial<CareerExperience>[]): CareerExperience[] {
  return entries.map((e) => ({
    id: e.id ?? '',
    company: e.company ?? '',
    title: e.title ?? '',
    startDate: e.startDate ?? '',
    endDate: e.endDate ?? '',
    description: e.description ?? '',
  }))
}

export async function fetchExperience(personaId: string): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ experience: Partial<CareerExperience>[] }>(res, 'load experience')
  return normalizeExperience(data.experience)
}

export async function addExperience(personaId: string, entry: Omit<CareerExperience, 'id'>): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...entry }),
  })
  const data = await jsonOrThrow<{ experience: Partial<CareerExperience>[] }>(res, 'add experience')
  return normalizeExperience(data.experience)
}

export type UpdateExperienceArgs = Partial<Omit<CareerExperience, 'id'>> & { id: string }

export async function updateExperience(personaId: string, args: UpdateExperienceArgs): Promise<CareerExperience[]> {
  const res = await fetch(`${PROXY_BASE}/experience`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ personaId, ...args }),
  })
  const data = await jsonOrThrow<{ experience: Partial<CareerExperience>[] }>(res, 'update experience')
  return normalizeExperience(data.experience)
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
  companyIds?: string[],
  portalId?: string,
  limit?: number,
  offset?: number,
): Promise<JobsResult> {
  const params = new URLSearchParams()
  if (query) params.set('query', query)
  if (location) params.set('location', location)
  // Repeated companyId params (?companyId=A&companyId=B) — the backend's
  // own jobsHandler reads every value for the key, not just the first,
  // since the company filter is multi-select.
  for (const id of companyIds ?? []) {
    if (id) params.append('companyId', id)
  }
  if (portalId) params.set('portalId', portalId)
  if (limit !== undefined) params.set('limit', String(limit))
  if (offset !== undefined) params.set('offset', String(offset))
  const qs = params.toString()
  const res = await fetch(`${PROXY_BASE}/jobs${qs ? `?${qs}` : ''}`, { credentials: 'include' })
  return jsonOrThrow<JobsResult>(res, 'load jobs')
}

// fetchJob reads a single job by id — GET /jobs?id=... short-circuits
// server-side (jobs.go's own getJobByID) to a single-job read instead
// of the paged list fetchJobs above returns. Used by JobDetailsPage,
// reached via the Jobs page's own Eye icon. Throws a plain Error on a
// 404 (unknown job id) same as every other not-found case in this
// file — the caller decides how to display it.
export async function fetchJob(id: string): Promise<Job> {
  const res = await fetch(`${PROXY_BASE}/jobs?id=${encodeURIComponent(id)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ job: Job }>(res, 'load job')
  return data.job
}

// The Jobs page's statistics strip (GET /jobs/stats): totals, what was added
// recently, how many jobs were matched (and the average), how many have a CV, the
// last 14 days and where the jobs come from. Validated with parseJobStats.
export async function fetchJobStatsRaw(): Promise<unknown> {
  const res = await fetch(`${PROXY_BASE}/jobs/stats`, { credentials: 'include' })
  return jsonOrThrow<unknown>(res, 'load job statistics')
}

// The one AI platform and model the whole tool uses (Settings page). Empty strings
// mean "not chosen": callers then fall back to the first platform that has a key
// (see Cinqo/Platform/aiChoice.ts).
export interface AiSettings {
  platformId: string
  model: string
}

export async function fetchAiSettings(): Promise<AiSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/ai`, { credentials: 'include' })
  return jsonOrThrow<AiSettings>(res, 'load the AI settings')
}

export async function saveAiSettings(settings: AiSettings): Promise<AiSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/ai`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify(settings),
  })
  return jsonOrThrow<AiSettings>(res, 'save the AI settings')
}

export async function fetchJobLocations(): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/job-locations`, { credentials: 'include' })
  const data = await jsonOrThrow<{ locations: string[] }>(res, 'load job locations')
  return data.locations
}

// --- Adding a job by hand (step 97) ---

export interface NewJobInput {
  title: string
  company?: string
  location?: string
  description?: string
  postedAt?: string
  sourceUrl?: string
}

// DuplicateJobError is the server's 409: the job already exists. existingJobId
// lets the page open it.
export class DuplicateJobError extends Error {
  readonly existingJobId: string
  constructor(message: string, existingJobId: string) {
    super(message)
    this.name = 'DuplicateJobError'
    this.existingJobId = existingJobId
  }
}

export async function addJob(input: NewJobInput): Promise<Job> {
  const res = await fetch(`${PROXY_BASE}/jobs`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(input),
  })
  if (res.status === 409) {
    const body = (await res.json().catch(() => ({}))) as { message?: string; existingJobId?: string }
    throw new DuplicateJobError(body.message ?? 'A job like this already exists', body.existingJobId ?? '')
  }
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text.trim() || `failed to add job (${res.status})`)
  }
  const data = await jsonOrThrow<{ job: Job }>(res, 'add job')
  return data.job
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

// updateJobMatch tracks a "Job Match" attempt's own lifecycle —
// starting one (status: 'matching', profileId + conversationId set)
// or recording a client-observed failure (status: 'failed', error
// set). Deliberately cannot set score or a persona — those are
// written exclusively by the AI's own save_job_match tool call
// (tools/career/backend/job_match.go), never by this endpoint. See
// plan/ai/tools/career/step-XX-job-match.md.
export async function updateJobMatch(
  jobId: string,
  args: { profileId?: string; status: 'matching' | 'failed'; conversationId?: string; error?: string },
): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs/match`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ jobId, ...args }),
  })
  if (!res.ok && res.status !== 204) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `failed to update job match (${res.status})`)
  }
}

// updateJobCv tracks a "Generate CV PDF" attempt's own lifecycle —
// starting one (status: 'generating', profileId + conversationId set)
// or recording a client-observed failure (status: 'failed', error
// set). Mirrors updateJobMatch exactly. A successful render is never
// recorded here — the AI's own save_cv_document MCP tool call, and the
// platform's own conversation orchestrator, settle cvStatus at
// 'completed' entirely server-side, with no frontend involvement. See
// plan/ai/tools/career/step-XX-cv-pdf.md.
export async function updateJobCv(
  jobId: string,
  args: { profileId?: string; status: 'generating' | 'failed'; conversationId?: string; error?: string },
): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs/cv`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ jobId, ...args }),
  })
  if (!res.ok && res.status !== 204) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `failed to update job cv (${res.status})`)
  }
}

// Attach an existing CV document to a job as its current CV (replacing any
// other): source "manual" for one just built in the CV Builder for this job,
// "existing" for one picked from the library. See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
export async function linkJobCv(jobId: string, cvDocumentId: string, source: 'manual' | 'existing'): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs/cv-link`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ jobId, cvDocumentId, source }),
  })
  if (!res.ok && res.status !== 204) {
    const text = await res.text().catch(() => '')
    throw new Error(text.trim() || `failed to attach the CV (${res.status})`)
  }
}

// Remove the job's current CV; the CV document and its PDF stay.
export async function unlinkJobCv(jobId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs/cv-link?jobId=${encodeURIComponent(jobId)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) {
    const text = await res.text().catch(() => '')
    throw new Error(text.trim() || `failed to remove the CV (${res.status})`)
  }
}

// mediaDownloadUrl builds a plain, clickable link to a Media file's
// own bytes — a core platform route (GET /api/v1/media/{id}/download),
// not one of this tool's own PROXY_BASE routes: Media is core-owned
// storage every tool shares, not something namespaced under any one
// tool's own proxy path.
export function mediaDownloadUrl(mediaFileId: string): string {
  return `/api/v1/media/${encodeURIComponent(mediaFileId)}/download`
}

// --- profile image ---
//
// A profile has at most one photo, stored via the core Media feature
// and linked from profiles.image_media_file_id (career.db) — always
// tied to a profile, never uploaded standalone. Re-uploading replaces
// it in place (same media file id, new bytes) rather than creating a
// second one.

export interface ProfileImage {
  profileId: string
  mediaFileId: string
}

// 404 (no image set yet) resolves to null — an expected, common state
// for this UI, not an error condition worth throwing over.
export async function fetchProfileImage(profileId: string): Promise<ProfileImage | null> {
  const res = await fetch(`${PROXY_BASE}/profile-image?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  if (res.status === 404) return null
  return jsonOrThrow<ProfileImage>(res, 'load profile image')
}

export async function uploadProfileImage(profileId: string, file: File): Promise<ProfileImage> {
  const formData = new FormData()
  formData.append('profileId', profileId)
  formData.append('file', file)
  const res = await fetch(`${PROXY_BASE}/profile-image`, {
    method: 'POST',
    credentials: 'include',
    body: formData,
  })
  return jsonOrThrow<ProfileImage>(res, 'upload profile image')
}

export async function deleteProfileImage(profileId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profile-image?profileId=${encodeURIComponent(profileId)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(`failed to delete profile image (${res.status})`)
  }
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
  args: {
    url?: string
    title?: string
    crawlInstructions?: string
    jobDetailCrawlInstructions?: string
    instructionsAiError?: string
    instructionsAiConversationId?: string
    jobDetailInstructionsAiError?: string
    jobDetailInstructionsAiConversationId?: string
    listingCrawlRequested?: boolean
    jobDetailInstructionsAiRequested?: boolean
    instructionsAiErrorDismissedAt?: string
    jobDetailInstructionsAiErrorDismissedAt?: string
    jobDetailCrawlRequested?: boolean
    autoDiscoveryEnabled?: boolean
    // autoDiscoveryIntervalMinutes/clearAutoDiscoveryInterval (step 85)
    // — set the former to a real value, or send the latter as true to
    // clear it back to "use global." Sending both is meaningless (the
    // backend's own PUT handler treats a real value as taking
    // precedence) — callers should only ever send one or the other.
    autoDiscoveryIntervalMinutes?: number
    clearAutoDiscoveryInterval?: boolean
    // emptyCrawlAcknowledged (step 90) — true marks this link's current
    // run of zero-job crawls as expected ("this page has no jobs"),
    // false clears it.
    emptyCrawlAcknowledged?: boolean
    // jobsQuestionAnswer (step 91) — answers the pending question. Both
    // clear it; "no" also marks the zero-job streak as expected.
    jobsQuestionAnswer?: 'yes' | 'no'
  },
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

// startInstructionAttempt/finishInstructionAttempt (step 86) record one
// "Generate with AI" run against a link's instruction counters. Both
// are idempotent server-side (a repeated start returns the running
// attempt; a finish with nothing running is a no-op), and the OUTCOME
// is decided by the server — finish only passes the failure reason to
// show if the run did not end up saving instructions.
export async function startInstructionAttempt(portalLinkId: string, kind: InstructionKind, triggeredBy: 'manual' | 'auto'): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portal-links/instruction-attempts/start`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalLinkId, kind, triggeredBy }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `failed to start instruction attempt (${res.status})`)
  }
}

// InstructionAttemptOutcome is how the backend ended a run (step 95): 'succeeded'
// when an AI save tool saved instructions during it — whatever the turn itself
// reported — 'failed' when it did not, 'none' when no attempt was running.
export type InstructionAttemptOutcome = 'succeeded' | 'failed' | 'none'

export async function finishInstructionAttempt(
  portalLinkId: string,
  kind: InstructionKind,
  error?: string,
): Promise<InstructionAttemptOutcome | undefined> {
  const res = await fetch(`${PROXY_BASE}/portal-links/instruction-attempts/finish`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalLinkId, kind, error }),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `failed to finish instruction attempt (${res.status})`)
  }
  const body = (await res.json().catch(() => ({}))) as { status?: string }
  return body.status === 'succeeded' || body.status === 'failed' || body.status === 'none' ? body.status : undefined
}

export async function removePortalLink(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portal-links?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove portal link (${res.status})`)
}

// --- CV import ---

export interface CVUploadResult {
  // Opaque id from the core Media feature — the frontend embeds this as
  // "media:<fileId>" into the AI's own initial instruction message
  // (buildImportPrompt.ts). Never a fetchable URL: pdf_to_markdown's
  // own url argument is resolved server-side, in-process, from this id
  // before the tool is ever invoked, so there is no HTTP fetch (and no
  // possible 401) for the AI's own reference to the CV.
  fileId: string
}

export async function uploadCV(file: File): Promise<CVUploadResult> {
  const formData = new FormData()
  formData.append('cv', file)
  const res = await fetch(`${PROXY_BASE}/cv-import/upload`, {
    method: 'POST',
    credentials: 'include',
    body: formData,
  })
  return jsonOrThrow<CVUploadResult>(res, 'upload cv')
}

// One completed CV analysis: the AI's own proposal, and — once the
// user acts on it — what was actually saved. aiProposal/saveSummary
// are kept as `unknown` here deliberately — api.ts stays decoupled
// from Components/ImportCv's own CVImportProposal/InsertResult shapes
// (matching this file's own "no cross-imports" convention); the caller
// casts to whichever shape it actually needs. saveSummary is null
// until the user's first "Insert" attempt for this run.
export interface CVImportRun {
  id: string
  mediaFileId: string
  originalFilename: string
  conversationId: string
  aiProposal: unknown
  saveSummary: unknown
  createdAt: string
  updatedAt: string
}

export async function fetchCVImportRuns(): Promise<CVImportRun[]> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs`, {
    method: 'GET',
    credentials: 'include',
  })
  const body = await jsonOrThrow<{ runs: CVImportRun[] }>(res, 'list cv import runs')
  return body.runs
}

export async function createCVImportRun(params: {
  fileId: string
  originalFilename: string
  conversationId: string
  aiProposal: unknown
}): Promise<string> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
  const body = await jsonOrThrow<{ id: string }>(res, 'create cv import run')
  return body.id
}

export async function updateCVImportRunSaveSummary(id: string, saveSummary: unknown): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, saveSummary }),
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to update cv import run (${res.status})`)
}

export async function deleteCVImportRun(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete cv import run (${res.status})`)
}

// --- cv documents (CV Builder) ---
//
// Deterministic (no AI), manual "pick a persona, pick a design, get a
// PDF" library — distinct from cvStatus/cvMediaFileId above (the
// AI-tailored per-job CV flow). CvData is this feature's own "JSON
// schema" — the exact shape every template renders against, mirroring
// tools/career/backend/cvbuilder/templates.CvData field-for-field. See
// plan/ai/career/cv-builder/step-01-overview-and-data-model.md.
export interface CvExternalLink {
  platform: string
  url: string
}

export interface CvExperienceEntry {
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

export interface CvData {
  fullName: string
  headline: string
  summary: string
  location: string
  externalLinks: CvExternalLink[]
  skills: string[]
  experience: CvExperienceEntry[]
}

export interface CvTemplate {
  id: string
  name: string
  description: string
  category: string
  bestFor: string[]
}

export interface CvDocument {
  id: string
  personaId: string
  templateId: string
  title: string
  data: CvData
  mediaFileId: string
  // Set only for a CV the AI generated for a specific job (Jobs page's
  // own "Generate CV PDF"); absent for one created through this
  // wizard. See plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
  jobId?: string
  // Filled only by fetchAllCvDocuments (the "Select existing" modal's list).
  personaName?: string
  createdAt: string
  updatedAt?: string
}

export async function fetchCvTemplates(): Promise<CvTemplate[]> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/templates`, { credentials: 'include' })
  const body = await jsonOrThrow<{ templates: CvTemplate[] }>(res, 'load cv templates')
  return body.templates
}

// The DEFAULT CvData a new CV starts from — the builder's own sidebar
// lets the user edit this freely afterward; nothing typed there ever
// writes back to the persona's own data.
export async function fetchCvPersonaDefaults(personaId: string): Promise<CvData> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/persona-defaults?personaId=${encodeURIComponent(personaId)}`, {
    credentials: 'include',
  })
  return jsonOrThrow<CvData>(res, 'load persona defaults')
}

// Answers "does this persona's own profile have a photo uploaded?" —
// a separate, deliberately tiny call rather than folding it into
// fetchCvPersonaDefaults: this is also called from the "edit an
// existing CV" load path, which must NOT re-fetch/overwrite that
// document's own already-loaded CvData the way persona-defaults would.
export async function fetchCvPhotoStatus(personaId: string): Promise<{ hasPhoto: boolean }> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/photo-status?personaId=${encodeURIComponent(personaId)}`, {
    credentials: 'include',
  })
  return jsonOrThrow<{ hasPhoto: boolean }>(res, 'check profile photo')
}

export async function fetchCvDocuments(personaId: string): Promise<CvDocument[]> {
  const res = await fetch(`${PROXY_BASE}/cv-documents?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const body = await jsonOrThrow<{ cvDocuments: CvDocument[] }>(res, 'load cv documents')
  return body.cvDocuments
}

// Used by the "edit this CV" flow — loads a document's own stored
// data (not the persona's current live data) back into the builder.
export async function fetchCvDocument(id: string): Promise<CvDocument> {
  const res = await fetch(`${PROXY_BASE}/cv-documents?id=${encodeURIComponent(id)}`, { credentials: 'include' })
  return jsonOrThrow<CvDocument>(res, 'load cv document')
}

// Renders templateId with data (the builder's own current, possibly
// edited state — never re-derived from personaId server-side), generates
// the PDF, uploads it to Media, and records the row.
export async function createCvDocument(
  personaId: string,
  templateId: string,
  title: string,
  data: CvData,
  attachPhoto: boolean,
): Promise<CvDocument> {
  const res = await fetch(`${PROXY_BASE}/cv-documents`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ personaId, templateId, title, data, attachPhoto }),
  })
  return jsonOrThrow<CvDocument>(res, 'generate cv document')
}

// Renders + generates a real PDF, same as createCvDocument's own first
// half, but creates NOTHING permanent — no Media file, no cv_documents
// row. Lets the wizard show what a CV would actually look like before
// the user commits to creating it. previewUrl is a temporary
// pdf_tools-owned URL (never a Media/cv_documents reference).
export async function previewCvDocument(
  personaId: string,
  templateId: string,
  data: CvData,
  attachPhoto: boolean,
): Promise<{ previewUrl: string }> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/preview`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ personaId, templateId, data, attachPhoto }),
  })
  return jsonOrThrow<{ previewUrl: string }>(res, 'preview cv document')
}

export async function updateCvDocumentTitle(id: string, title: string): Promise<CvDocument> {
  const res = await fetch(`${PROXY_BASE}/cv-documents`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, title }),
  })
  return jsonOrThrow<CvDocument>(res, 'update cv document title')
}

// A job that uses a CV document, as the delete confirmation lists it.
export interface CvDocumentJob {
  id: string
  title: string
  company: string
}

// Thrown by deleteCvDocument when jobs use the CV and the caller has not yet
// confirmed: nothing was deleted, and jobs lists EVERY job that uses it.
export class CvDocumentInUseError extends Error {
  jobs: CvDocumentJob[]
  constructor(jobs: CvDocumentJob[]) {
    super('this CV is used by jobs')
    this.jobs = jobs
  }
}

// unlinkJobs: true is the confirmation — the jobs lose their link, then the CV is
// deleted. Without it, a CV that jobs use answers 409 and is left alone
// (CvDocumentInUseError). See plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
export async function deleteCvDocument(id: string, opts: { unlinkJobs?: boolean } = {}): Promise<void> {
  const query = `id=${encodeURIComponent(id)}${opts.unlinkJobs ? '&unlinkJobs=true' : ''}`
  const res = await fetch(`${PROXY_BASE}/cv-documents?${query}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (res.status === 409) {
    const body = (await res.json().catch(() => null)) as { error?: string; jobs?: CvDocumentJob[] } | null
    if (body?.error === 'in_use' && Array.isArray(body.jobs)) throw new CvDocumentInUseError(body.jobs)
  }
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete cv document (${res.status})`)
}

// Every persona's CV documents, newest first (at most 200), each with its
// persona's name — what the Jobs page's "Select existing" modal picks from.
export async function fetchAllCvDocuments(): Promise<CvDocument[]> {
  const res = await fetch(`${PROXY_BASE}/cv-documents?all=1`, { credentials: 'include' })
  const body = await jsonOrThrow<{ cvDocuments: CvDocument[] }>(res, 'load cv documents')
  return body.cvDocuments
}
