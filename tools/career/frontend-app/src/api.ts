// api.ts — thin fetch helpers over this tool's own proxy routes
import type { DeleteAnswer } from './Components/JobsPage/deleteRules'
import type { CrawlerSettings, EditableCrawlerSettings, IgnoredList } from './Components/SettingsPage/crawlerRules'
import type { BatchStatus, SkippedJob } from './Components/JobsPage/recrawlRules'
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
  // The Media file id of the profile's photo; missing when it has none.
  imageMediaFileId?: string
}

export interface Persona {
  id: string
  profileId: string
  name: string
  description?: string
  createdAt: string
  updatedAt?: string
  // The "create personas with AI" run this persona came from; missing otherwise (step 112).
  aiRunId?: string
}

// --- create personas with AI (step 112) ---
// A run is one AI attempt of a profile: the hidden conversation, what the user asked for, how it
// ended and the AI's proposal. See plan/ai/tools/career/step-112-create-personas-with-ai.md.

export interface ProposedRole {
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

export interface ProposedPersona {
  name: string
  description: string
  // The id of an existing persona this one looks like, or null.
  possibleDuplicateOf: string | null
  details: {
    headline: string
    summary: string
    location: string
    desiredTitles: string
    desiredLocations: string
    minSalary: number
  }
  skills: string[]
  experience: ProposedRole[]
}

export interface PersonaProposal {
  personas: ProposedPersona[]
  note: string
}

export type PersonaAiRunStatus = 'generating' | 'ready' | 'failed'

export interface PersonaAiRun {
  id: string
  profileId: string
  conversationId: string
  focus: string
  status: PersonaAiRunStatus
  // Only once ready.
  proposal?: PersonaProposal
  error?: string
  createdAt: string
  finishedAt?: string
  // The personas created from this run; empty until the proposal is decided (and when every
  // persona was declined).
  personas: { id: string; name: string }[]
  // When the user decided on the whole proposal (missing until then), and the proposed personas
  // they declined (step 113).
  decidedAt?: string
  // The indexes of the proposed personas already decided (accepted or declined), one at a time.
  decided: number[]
  // The id of each suggestion the AI stored, in proposal order (what its chat links point at).
  suggestionIds: string[]
  declined: { name: string }[]
}

// The user's decision on the proposed persona at `index` (its position in the proposal): accepted
// with the final, possibly edited content, or declined with none.
export type PersonaDecision = { index: number; decision: 'accepted'; persona: ProposedPersona } | { index: number; decision: 'declined' }

export async function fetchPersonaAiRuns(profileId: string): Promise<PersonaAiRun[]> {
  const res = await fetch(`${PROXY_BASE}/persona-ai-runs?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ runs: PersonaAiRun[] }>(res, 'load persona AI runs')
  return data.runs
}

// Records the start of a run, before the first message is sent, so a reload can resume it.
export async function createPersonaAiRun(profileId: string, conversationId: string, focus: string): Promise<string> {
  const res = await fetch(`${PROXY_BASE}/persona-ai-runs`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ profileId, conversationId, focus }),
  })
  const data = await jsonOrThrow<{ id: string }>(res, 'record persona AI run')
  return data.id
}

// Records the user's decision on EVERY proposed persona of a ready run and creates the accepted ones
// (possibly edited) in one go, all or none. The run is closed afterwards.
export async function applyPersonaAiRun(runId: string, decisions: PersonaDecision[]): Promise<{ id: string; name: string }[]> {
  const res = await fetch(`${PROXY_BASE}/persona-ai-runs/apply`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ runId, decisions }),
  })
  if (!res.ok) {
    throw new Error((await res.text()).trim() || `failed to create the personas (${res.status})`)
  }
  const data = (await res.json()) as { personas: { id: string; name: string }[] }
  return data.personas
}

// Removes a run from the history. The personas created from it stay.
export async function deletePersonaAiRun(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/persona-ai-runs?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) {
    throw new Error(`failed to remove the run (${res.status})`)
  }
}

// Ends a generating run: ready (the proposal is the suggestions the AI stored with its
// suggest_persona tool), or failed with a reason.
export async function finishPersonaAiRun(id: string, outcome: { status: 'ready' } | { status: 'failed'; error: string }): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/persona-ai-runs`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, ...outcome }),
  })
  if (!res.ok) {
    throw new Error((await res.text()).trim() || `failed to finish persona AI run (${res.status})`)
  }
}

export interface PersonaDetails {
  personaId: string
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
  // Whether the persona wants remote work (step 130): '' (not set), 'onsite', 'open' or 'remote_only'.
  remotePreference?: '' | 'onsite' | 'open' | 'remote_only'
  // The values among summary, desiredTitles, desiredLocations, minSalary and remotePreference that are the persona's OWN (plan 174); the
  // others are the profile's, which the persona follows. Missing on an answer that does not say.
  overridden?: string[]
}

export interface CareerExperience {
  id: string
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
  // Where the entry of a persona comes from (plan 173): the profile's library (the persona uses it) or the persona itself; missing on an
  // answer that does not say.
  source?: 'profile' | 'persona'
  // The persona shows its own title or description instead of the library's.
  titleOverridden?: boolean
  descriptionOverridden?: boolean
}

// A skill of a persona with where it comes from.
export interface PersonaSkillItem {
  id: string
  skill: string
  source: 'profile' | 'persona'
}

export interface PersonaDetailsResult {
  personaDetails: PersonaDetails | null
  skills: string[]
  skillItems?: PersonaSkillItem[]
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
  portalLinkId?: string
  // canRecrawl (step 148): the job has a posting URL and a portal link with job detail crawl instructions, so it can be recrawled
  // for a fresh description. Absent from a backend that predates it.
  canRecrawl?: boolean
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
  // matchKind (step 129) is the engine that made the match, 'ai' or 'manual'; matchBreakdown is the manual
  // engine's stored result (absent for an AI result and for a job without a match).
  matchKind?: 'ai' | 'manual'
  matchBreakdown?: ManualMatchResult
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
  // boardProfileCount (step 126) — on how many profiles' job boards this job is; always sent, 0 when none.
  boardProfileCount: number
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
  // The company linked first, or "" (kept for older callers); companyIds/companies are all of them, none is possible (step 169).
  companyId: string
  companyIds: string[]
  companies: { id: string; name: string }[]
  // A freelancer is independent on purpose and attached to no company (plan step-171); never together with companies.
  freelancer: boolean
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

// LinkRule (plan 185) is what a person's picked job links became: the host, the patterns that say "this link is a job" and the
// ones that say "this one is not" (exclusions win).
export interface LinkRule {
  host: string
  include: string[]
  exclude?: string[]
  // plan 203: the one other site of this link's job links, set only from the link's confirmed job site (never typed into a rule).
  jobHost?: string
  // Where the pages after the first are (absent = the first page only).
  pagination?: LinkPagination
}

// kind 'template': page k >= 2 is `template` with {n} = second + (k-2)*step; kind 'link': the next page is the link of the current page
// that matches `pattern`. maxPages: the most pages one crawl loads (absent = 5, at most 50).
export interface LinkPagination {
  kind: 'none' | 'template' | 'link'
  template?: string
  second?: number
  step?: number
  pattern?: string
  maxPages?: number
}

export interface PortalLink {
  id: string
  // This link's own AI platform and model; "" means none set (the portal's, then the
  // tool-wide, choice applies). Always sent by the backend, never missing.
  aiPlatformId: string
  aiModel: string
  portalId: string
  url: string
  title?: string | null
  crawlInstructions?: string | null
  // Separate, second instruction document — how to extract job-
  // position-relevant text off a single job's own detail page, rather
  // than crawlInstructions' own listing-page fields+pagination shape.
  // See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
  jobDetailCrawlInstructions?: string | null
  // Who wrote the job-detail instructions (plan 186, step 9): 'picked' (the picking screen), 'ai', 'manual' (typed in the text box),
  // 'inherited' (copied from another link) or '' (saved before it was recorded). Always sent by the backend.
  jobDetailInstructionsOrigin: string
  // Separate, job-detail-specific counterparts to
  // instructionsAiError/instructionsAiErrorAt/instructionsAiConversationId
  // below, which track the LISTING instructions' own AI generation
  // instead. See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
  jobDetailInstructionsAiError?: string | null
  jobDetailInstructionsAiErrorAt?: string | null
  jobDetailInstructionsAiConversationId?: string | null
  // Most recent job crawled_at against this link (step 26) — null
  // when no job has ever been saved for it. Read-only, derived.
  lastCrawledAt?: string | null
  // Most recent failure of an AI-driven crawl-instructions generation/
  // edit attempt for this link (step 33) — both null when no failure
  // is currently recorded. Read-only; written only via
  // updatePortalLink's own instructionsAiError field.
  instructionsAiError?: string | null
  instructionsAiErrorAt?: string | null
  // The hidden conversation currently generating/updating this link's
  // own crawl instructions, if one is in flight (step 60) — null once
  // it finishes (success or failure). Read-only; written only via
  // updatePortalLink's own instructionsAiConversationId field. See
  // plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
  instructionsAiConversationId?: string | null
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
  // requiresLogin/loginUrl (plan 182) are a link SETTING, never a login: this page needs the person to log in (in a window the Browser tool
  // opens for each crawl), and the page to start at ('' = the link's own address). Always sent by the backend.
  requiresLogin: boolean
  loginUrl: string
  // loginSession (plan 195, step 4): whether a login kept in the Browser tool is available for this link ('none', 'valid', 'expired'). Never
  // the session itself. Always sent by the backend.
  loginSession: 'none' | 'valid' | 'expired'
  // crawlEngine (plan 197): this link's own engine; '' follows the Browser tool's selected engine, 'rod' reads this link the Rod way. Always sent.
  crawlEngine: '' | 'rod'
  // jobHost (plan 203): the one other site a person confirmed this link's job links are on ('' = none). Always sent by the backend.
  jobHost: string
  // engineSuggestion (plan 197, step 4): set while a Katana read of this link was stopped by the site and the person has not said "Not now" since.
  engineSuggestion: { code: string; since: string } | null
  // linkRules (plan 185): the link crawler's rule (null = none). Which of the rule and the AI instructions a link is crawled with is decided
  // by the Browser's selected engine (plan 190), not by a stored choice: the backend still stores and sends a crawl method, nothing reads it.
  linkRules: LinkRule | null
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
  // This portal's own AI platform and model for crawling; "" means none set (the
  // tool-wide choice applies). Always sent by the backend, never missing.
  aiPlatformId: string
  aiModel: string
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
  remotePreference?: '' | 'onsite' | 'open' | 'remote_only'
  // Values to send back to following the profile's (plan 174): any of summary, desiredTitles, desiredLocations, minSalary, remotePreference.
  followProfile?: string[]
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
    source: e.source,
    titleOverridden: e.titleOverridden,
    descriptionOverridden: e.descriptionOverridden,
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

// How the jobs list is ordered: newest first (the default) or best match first (step 129).
export type JobsSort = 'newest' | 'match'

export async function fetchJobs(
  query: string,
  location: string,
  companyIds?: string[],
  portalId?: string,
  limit?: number,
  offset?: number,
  sort?: JobsSort,
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
  if (sort === 'match') params.set('sort', sort)
  const qs = params.toString()
  const res = await fetch(`${PROXY_BASE}/jobs${qs ? `?${qs}` : ''}`, { credentials: 'include' })
  const result = await jsonOrThrow<JobsResult>(res, 'load jobs')
  // The backend leaves out what a job does not have (a job found by the link crawler has only a title and an address): the page reads these
  // as text, so what is absent is an empty text here, once, instead of a crash in every place that trims or compares it.
  return { ...result, jobs: result.jobs.map(withTexts) }
}

export function withTexts(job: Job): Job {
  // What the backend left out is read as it really is (the wire type says what a job usually has, not what every job has).
  const raw: Partial<Job> = job
  return {
    ...job,
    title: raw.title ?? '',
    company: raw.company ?? '',
    sourceUrl: raw.sourceUrl ?? '',
    postedAt: raw.postedAt ?? '',
    crawledAt: raw.crawledAt ?? '',
  }
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
// mean "not chosen": callers then fall back to the best usable platform (the local AI first, then one with a key)
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

// Job cleanup (Settings → Job cleanup, step 131): the two rules, the automatic run, a preview and a real run.
export interface CleanupSettings {
  enabled: boolean
  olderEnabled: boolean
  olderThanDays: number
  lowMatchEnabled: boolean
  minMatchPercent: number
  includeManual: boolean
  intervalHours: number
  lastRunAt: string
  lastRunDeleted: number
  lastRunSummary: string
}

export type CleanupRule = 'older' | 'low_match' | 'both'

// What the page sends for a preview or a run. The numbers are the ones typed in the form (saved or not), so a
// preview shows what that form would do; confirm is only for a run.
export interface CleanupRequest {
  rule: CleanupRule
  olderThanDays: number
  minMatchPercent: number
  includeManual: boolean
  confirm?: boolean
}

export interface CleanupSample {
  id: string
  title: string
  company: string
  crawledAt: string
  bestPercent?: number
}

export interface CleanupPreview {
  totalJobs: number
  wouldDelete: number
  byRule: { older: number; lowMatch: number }
  kept: { onBoard: number; hasCv: number; inProgress: number; manual: number; unjudged: number }
  sample: CleanupSample[]
}

export interface CleanupRunResult {
  deleted: number
  byRule: { older: number; lowMatch: number }
  durationMs: number
}

export async function fetchCleanupSettings(): Promise<CleanupSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/cleanup`, { credentials: 'include' })
  return jsonOrThrow<CleanupSettings>(res, 'load the cleanup settings')
}

export async function saveCleanupSettings(
  settings: Omit<CleanupSettings, 'lastRunAt' | 'lastRunDeleted' | 'lastRunSummary'>,
): Promise<CleanupSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/cleanup`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  return jsonOrThrow<CleanupSettings>(res, 'save the cleanup settings')
}

// The Job crawler's "Ignore jobs" settings (step 150): the switch, the rule and its limit.
export async function fetchCrawlerSettings(): Promise<CrawlerSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/crawler`, { credentials: 'include' })
  return jsonOrThrow<CrawlerSettings>(res, 'load the crawler settings')
}

export async function saveCrawlerSettings(settings: EditableCrawlerSettings): Promise<CrawlerSettings> {
  const res = await fetch(`${PROXY_BASE}/settings/crawler`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  return jsonOrThrow<CrawlerSettings>(res, 'save the crawler settings')
}

// The latest jobs the rules kept out (newest first, at most `limit`; the log keeps 200).
export async function fetchIgnoredJobs(limit = 50): Promise<IgnoredList> {
  const res = await fetch(`${PROXY_BASE}/settings/crawler/ignored?limit=${String(limit)}`, { credentials: 'include' })
  return jsonOrThrow<IgnoredList>(res, 'load the jobs the crawler kept out')
}

export async function previewCleanup(request: CleanupRequest): Promise<CleanupPreview> {
  const res = await fetch(`${PROXY_BASE}/jobs/cleanup/preview`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  })
  return jsonOrThrow<CleanupPreview>(res, 'preview the cleanup')
}

export async function runCleanup(request: CleanupRequest): Promise<CleanupRunResult> {
  const res = await fetch(`${PROXY_BASE}/jobs/cleanup/run`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...request, confirm: true }),
  })
  return jsonOrThrow<CleanupRunResult>(res, 'delete the jobs')
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

// A batch delete that failed; deleted is how many jobs went before it stopped (0 when it failed before deleting any).
export class DeleteJobsError extends Error {
  readonly deleted: number
  constructor(message: string, deleted: number) {
    super(message)
    this.name = 'DeleteJobsError'
    this.deleted = deleted
  }
}

// Deletes the jobs in one request (step 149), however many there are; a crawled job is remembered so the crawler does not add it
// again. The server answers how many it deleted and how many ids it did not find.
export async function deleteJobs(jobIds: string[]): Promise<DeleteAnswer> {
  const res = await fetch(`${PROXY_BASE}/jobs/delete`, { method: 'POST', credentials: 'include', body: JSON.stringify({ jobIds }) })
  if (res.ok) return (await res.json()) as DeleteAnswer
  const text = await res.text()
  try {
    const body = JSON.parse(text) as { error?: string; deleted?: number }
    throw new DeleteJobsError(body.error ?? `failed to delete the jobs (${String(res.status)})`, body.deleted ?? 0)
  } catch (err) {
    if (err instanceof DeleteJobsError) throw err
    throw new DeleteJobsError(text.trim() || `failed to delete the jobs (${String(res.status)})`, 0)
  }
}

export async function removeJob(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove job (${res.status})`)
}

// The jobs the crawler will not add again (Settings → Job cleanup, step 133): the ones you deleted and the ones the
// low-match cleanup deleted. available is false when the tool could not open its deleted-jobs database.
export interface DeletedJob {
  id: number
  sourceUrl: string
  title: string
  company: string
  location: string
  portalName: string
  postedAt: string
  matchScore: number | null
  reason: 'deleted' | 'low_match'
  deletedAt: string
}

export interface DeletedJobsList {
  available: boolean
  total: number
  items: DeletedJob[]
}

export async function fetchDeletedJobs(limit = 50): Promise<DeletedJobsList> {
  const res = await fetch(`${PROXY_BASE}/jobs/deleted?limit=${String(limit)}`, { credentials: 'include' })
  return jsonOrThrow<DeletedJobsList>(res, 'load the deleted jobs')
}

// Allow one job again: the next crawl may add it as a new job. Nothing is brought back by this call.
export async function allowDeletedJob(id: number): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/jobs/deleted?id=${String(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) throw new Error(`failed to allow the job again (${String(res.status)})`)
}

export async function allowAllDeletedJobs(): Promise<number> {
  const res = await fetch(`${PROXY_BASE}/jobs/deleted?all=true`, { method: 'DELETE', credentials: 'include' })
  const data = await jsonOrThrow<{ released: number }>(res, 'allow the deleted jobs again')
  return data.released
}

// "Match unmatched jobs" (Jobs page, step 134): the manual match, no AI, for every job that has no match yet, run now.
// Every job it looked at is counted in exactly one of the other numbers; more is true when there were more unmatched
// jobs than one press handles.
export interface MatchUnmatchedResult {
  unmatched: number
  matched: number
  unjudged: number
  failed: number
  skippedExisting: number
  more: boolean
}

export async function matchUnmatchedJobs(): Promise<MatchUnmatchedResult> {
  const res = await fetch(`${PROXY_BASE}/jobs/match/unmatched`, { method: 'POST', credentials: 'include' })
  if (res.status === 409) throw new Error('Matching is already running. Try again in a moment.')
  return jsonOrThrow<MatchUnmatchedResult>(res, 'match the unmatched jobs')
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
// A photo is one file in the core Media feature, kept in the photo library (career.db photos
// table). A profile can have several photos (profile_photos) and one of them is its default
// (profiles.image_media_file_id); a photo can belong to many profiles. Uploading always makes a
// new photo and adds it to the profile (the default only when it has none). Removing a photo from
// a profile only takes it off that profile; a photo is deleted for good with deletePhoto. See
// plan/ai/tools/career/step-110-shared-photo-library.md and step-111-many-photos-per-profile.md.

export interface ProfileImage {
  profileId: string
  mediaFileId: string
}

export interface ProfilePhoto {
  mediaFileId: string
  isDefault: boolean
  addedAt: string
}

// What an upload answers: the photo that was added and whether it became the default.
export interface UploadedPhoto extends ProfileImage {
  isDefault: boolean
}

// The profile's photos, the default first, then the newest.
export async function fetchProfilePhotos(profileId: string): Promise<ProfilePhoto[]> {
  const res = await fetch(`${PROXY_BASE}/profile-photos?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ photos: ProfilePhoto[] }>(res, 'load profile photos')
  return data.photos
}

export async function uploadProfileImage(profileId: string, file: File): Promise<UploadedPhoto> {
  const formData = new FormData()
  formData.append('profileId', profileId)
  formData.append('file', file)
  const res = await fetch(`${PROXY_BASE}/profile-image`, {
    method: 'POST',
    credentials: 'include',
    body: formData,
  })
  return jsonOrThrow<UploadedPhoto>(res, 'upload profile image')
}

// Adds a photo that is already in the library to the profile (its default only if it has none).
export async function addProfilePhoto(profileId: string, mediaFileId: string): Promise<ProfilePhoto[]> {
  const res = await fetch(`${PROXY_BASE}/profile-photos`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ profileId, mediaFileId }),
  })
  const data = await jsonOrThrow<{ photos: ProfilePhoto[] }>(res, 'add photo')
  return data.photos
}

// Takes a photo off the profile; the photo stays in the library. If it was the default, the
// profile's most recently added remaining photo becomes the default.
export async function removeProfilePhoto(profileId: string, mediaFileId: string): Promise<void> {
  const res = await fetch(
    `${PROXY_BASE}/profile-photos?profileId=${encodeURIComponent(profileId)}&mediaFileId=${encodeURIComponent(mediaFileId)}`,
    { method: 'DELETE', credentials: 'include' },
  )
  if (!res.ok) {
    throw new Error(`failed to remove photo (${res.status})`)
  }
}

// Makes one of the profile's photos (or a library photo, which is then added) its default.
export async function makeDefaultPhoto(profileId: string, mediaFileId: string): Promise<ProfileImage> {
  const res = await fetch(`${PROXY_BASE}/profile-image`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ profileId, mediaFileId }),
  })
  return jsonOrThrow<ProfileImage>(res, 'set default photo')
}

export interface PhotoUser {
  profileId: string
  name: string
  isDefault: boolean
}

export interface Photo {
  mediaFileId: string
  createdAt: string
  usedBy: PhotoUser[]
}

export async function fetchPhotos(): Promise<Photo[]> {
  const res = await fetch(`${PROXY_BASE}/photos`, { credentials: 'include' })
  const data = await jsonOrThrow<{ photos: Photo[] }>(res, 'load photos')
  return data.photos
}

// Deletes a photo for good: every profile that has it loses it.
export async function deletePhoto(mediaFileId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/photos?mediaFileId=${encodeURIComponent(mediaFileId)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok) {
    throw new Error(`failed to delete photo (${res.status})`)
  }
}

// --- companies ---

export async function fetchCompanies(): Promise<Company[]> {
  const res = await fetch(`${PROXY_BASE}/companies`, { credentials: 'include' })
  const data = await jsonOrThrow<{ companies: Company[] }>(res, 'load companies')
  return data.companies
}

// The overview of ALL companies, whatever the search or the page (step 137).
export interface CompanyOverview {
  companies: number
  withJobs: number
  withRecruiters: number
  jobs: number
  recruiters: number
}

// One searched page of the Companies list: the companies on it, how many companies match the search (the pager's total) and
// the overview of all of them.
export interface CompaniesPage {
  companies: Company[]
  total: number
  summary: CompanyOverview
}

export async function fetchCompaniesPage(query: string, limit: number, offset: number): Promise<CompaniesPage> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (query !== '') params.set('query', query)
  const res = await fetch(`${PROXY_BASE}/companies?${params.toString()}`, { credentials: 'include' })
  return jsonOrThrow<CompaniesPage>(res, 'load companies')
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

export async function fetchRecruiters(companyId?: string, freelancersOnly = false): Promise<Recruiter[]> {
  const qs = freelancersOnly ? '?freelancer=true' : companyId ? `?companyId=${encodeURIComponent(companyId)}` : ''
  const res = await fetch(`${PROXY_BASE}/recruiters${qs}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ recruiters: Recruiter[] }>(res, 'load recruiters')
  return data.recruiters
}

// --- recruiters mapped to jobs (plan/ai/tools/career/step-168-recruiters-on-jobs.md) ---

// A recruiter mapped to one job, with their company's name and when the mapping was made.
export interface JobRecruiter {
  id: string
  firstName: string
  lastName: string
  email: string
  // The company linked first (or ""), kept for older callers; companies is all of them, none is possible (step 169).
  companyId: string
  companyName: string
  companies: { id: string; name: string }[]
  // A freelancer is independent on purpose and has no company (plan step-171).
  freelancer: boolean
  mappedAt: string
}

// A job a recruiter is mapped to.
export interface RecruiterJob {
  id: string
  title: string
  company?: string
  companyId?: string
  location?: string
  mappedAt: string
}

const UNKNOWN_MAPPING_MESSAGE = 'This job or recruiter does not exist (any more).'

// The recruiters mapped to a job; null when there is no such job (it was deleted, or the address is wrong).
export async function fetchJobRecruiters(jobId: string): Promise<JobRecruiter[] | null> {
  const res = await fetch(`${PROXY_BASE}/job-recruiters?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  if (res.status === 404) return null
  const data = await jsonOrThrow<{ recruiters: JobRecruiter[] }>(res, 'load the recruiters of the job')
  return data.recruiters
}

// The jobs a recruiter is mapped to; null when there is no such recruiter.
export async function fetchRecruiterJobs(recruiterId: string): Promise<RecruiterJob[] | null> {
  const res = await fetch(`${PROXY_BASE}/job-recruiters?recruiterId=${encodeURIComponent(recruiterId)}`, { credentials: 'include' })
  if (res.status === 404) return null
  const data = await jsonOrThrow<{ jobs: RecruiterJob[] }>(res, 'load the jobs of the recruiter')
  return data.jobs
}

// Maps a recruiter to a job (a repeat is not an error: mapped is then false). Answers with the job's recruiters.
export async function mapJobRecruiter(jobId: string, recruiterId: string): Promise<{ mapped: boolean; recruiters: JobRecruiter[] }> {
  const res = await fetch(`${PROXY_BASE}/job-recruiters`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ jobId, recruiterId }),
  })
  if (res.status === 404) throw new Error(UNKNOWN_MAPPING_MESSAGE)
  return jsonOrThrow<{ mapped: boolean; recruiters: JobRecruiter[] }>(res, 'map the recruiter to the job')
}

// Takes a recruiter off a job; fine when they were not mapped.
export async function unmapJobRecruiter(jobId: string, recruiterId: string): Promise<void> {
  const res = await fetch(
    `${PROXY_BASE}/job-recruiters?jobId=${encodeURIComponent(jobId)}&recruiterId=${encodeURIComponent(recruiterId)}`,
    {
      method: 'DELETE',
      credentials: 'include',
    },
  )
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove the recruiter from the job (${String(res.status)})`)
}

export async function createRecruiter(
  companyIds: string[],
  firstName: string,
  lastName: string,
  email: string,
  freelancer = false,
): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/recruiters`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify(freelancer ? { freelancer: true, firstName, lastName, email } : { companyIds, firstName, lastName, email }),
  })
  return jsonOrThrow<{ id: string }>(res, 'create recruiter')
}

// companyIds replaces the whole set of companies (an empty list removes them all); left out, the companies stay as they are.
// freelancer: true makes the recruiter a freelancer (the server removes every company); false with companyIds makes a company recruiter.
export async function updateRecruiter(
  id: string,
  args: { firstName?: string; lastName?: string; email?: string; companyIds?: string[]; freelancer?: boolean },
): Promise<Recruiter[]> {
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

// Where a link is in the chain from its listing instructions to a complete set of jobs (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md,
// step 1). Read-only, derived from data that exists; the words for a stage are in Crawler/pipelineStage.ts.
export type PipelineStageId =
  | 'listing_question'
  | 'listing_instructions_generating'
  | 'no_listing_instructions'
  | 'listing_crawling'
  | 'detail_instructions_generating'
  | 'detail_crawling'
  | 'listing_crawl_waiting'
  | 'listing_ready'
  | 'detail_instructions_requested'
  | 'detail_instructions_failed'
  | 'detail_instructions_missing'
  | 'detail_crawl_waiting'
  | 'detail_crawl_pending'
  | 'complete'

// What a link waits for: a person, an open Platforms page that acts on a request flag, an AI run, a crawl, or nothing (done).
export type PipelineWaitingFor = 'user' | 'page' | 'ai' | 'crawl' | 'nothing'

export interface PipelineLink {
  portalLinkId: string
  stage: PipelineStageId
  waitingFor: PipelineWaitingFor
  // When the running work of the stage started ("2026-11-02 10:00:00", UTC); absent for a stage that is not running.
  since?: string
  detail: { jobs: number; jobsWithDescription: number; detailFailedRuns: number }
}

export async function fetchPipeline(): Promise<PipelineLink[]> {
  const res = await fetch(`${PROXY_BASE}/portal-links/pipeline`, { credentials: 'include' })
  const data = await jsonOrThrow<{ links: PipelineLink[] }>(res, 'load the pipeline of the platform links')
  return data.links
}

export async function fetchPortals(): Promise<Portal[]> {
  const res = await fetch(`${PROXY_BASE}/portals`, { credentials: 'include' })
  const data = await jsonOrThrow<{ portals: Portal[] }>(res, 'load platforms')
  return data.portals
}

export async function createPortal(name: string): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/portals`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ name }),
  })
  return jsonOrThrow<{ id: string }>(res, 'create platform')
}

export async function updatePortal(id: string, name: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portals`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, name }),
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to update platform (${res.status})`)
}

// What POST /portal-links/start-fresh answers (step 161): the conversations of the link's earlier AI runs,
// for the caller to delete through the core, and how many attempt records were removed.
export interface StartFreshResult {
  conversationIds: string[]
  attemptsRemoved: number
}

// startFreshPortalLink wipes one link's earlier AI attempts and leftovers (and, only when asked, its stored crawl
// instructions). The server answers 409 while a generation is still running for the link; its text is the error.
export async function startFreshPortalLink(portalLinkId: string, clearInstructions: boolean): Promise<StartFreshResult> {
  const res = await fetch(`${PROXY_BASE}/portal-links/start-fresh`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalLinkId, clearInstructions }),
  })
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(res.status === 409 && text !== '' ? text : `failed to start fresh (${res.status})`)
  }
  const data = (await res.json()) as Partial<StartFreshResult>
  return { conversationIds: data.conversationIds ?? [], attemptsRemoved: data.attemptsRemoved ?? 0 }
}

export async function removePortal(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/portals?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok && res.status !== 204) throw new Error(`failed to delete platform (${res.status})`)
}

export async function addPortalLink(
  portalId: string,
  url: string,
  title: string,
  login?: { requiresLogin: boolean; loginUrl: string },
): Promise<{ id: string }> {
  const res = await fetch(`${PROXY_BASE}/portal-links`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalId, url, title, ...(login ?? {}) }),
  })
  return jsonOrThrow<{ id: string }>(res, 'add platform link')
}

// What POST /portal-links/inherit-instructions answers (step 103). The refreshed portal list is
// always in it; every other field may be missing, which the callers treat as "nothing".
export interface InheritInstructionsResult {
  inherited?: boolean
  listing?: boolean
  jobDetail?: boolean
  from?: { linkId?: string; title?: string } | null
  tried?: { linkId?: string; title?: string; listing?: string; jobDetail?: string; reason?: string }[] | null
  portals: Portal[]
}

// Tries the instructions of the portal's other links on a link that has none, and saves them
// on it when they work (the backend does the trial; nothing is saved otherwise).
export async function inheritPortalInstructions(id: string): Promise<InheritInstructionsResult> {
  const res = await fetch(`${PROXY_BASE}/portal-links/inherit-instructions`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ id }),
  })
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(text || `failed to try the platform's instructions (${res.status})`)
  }
  return (await res.json()) as InheritInstructionsResult
}

// Sets (or, with both empty, removes) a portal's own AI platform and model.
export async function setPortalAi(id: string, platformId: string, model: string): Promise<Portal[]> {
  const res = await fetch(`${PROXY_BASE}/portals/ai`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, platformId, model }),
  })
  const data = await jsonOrThrow<{ portals: Portal[] }>(res, "save the platform's AI choice")
  return data.portals
}

// Sets (or, with both empty, removes) a link's own AI platform and model.
export async function setPortalLinkAi(id: string, platformId: string, model: string): Promise<Portal[]> {
  const res = await fetch(`${PROXY_BASE}/portal-links/ai`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, platformId, model }),
  })
  const data = await jsonOrThrow<{ portals: Portal[] }>(res, "save the link's AI choice")
  return data.portals
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
    // plan 182: the link's login setting (never a login itself).
    requiresLogin?: boolean
    loginUrl?: string
    // plan 197: '' = follow the Browser tool's engine, 'rod' = this link is read the Rod way.
    crawlEngine?: '' | 'rod'
    // plan 203: the confirmed job site ('' removes it).
    jobHost?: string
    // plan 197: the person's "Not now" for the suggestion to use Rod (a newer block shows it again).
    engineHintDismissed?: boolean
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
    throw new Error(text || `failed to update platform link (${res.status})`)
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
  if (!res.ok && res.status !== 204) throw new Error(`failed to remove platform link (${res.status})`)
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

// The text of an uploaded CV, read by the Career backend with the same library and cleaning as the pdf_to_markdown tool (plan 209).
// Rejects with the server's own reason (the file is gone, not a PDF, no text).
export async function fetchCvText(fileId: string): Promise<string> {
  const res = await fetch(`${PROXY_BASE}/cv-import/text?fileId=${encodeURIComponent(fileId)}`, { credentials: 'include' })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to read the CV text (${String(res.status)})`)
  return ((await res.json()) as { text: string }).text
}

// Puts one piece on the draft of a generating run, exactly as the cv_import_* tools do (same checks). Rejects with the server's reason when
// the piece is refused (400), the run is unknown (404) or no longer generating (409).
export async function storeDraftPiece(runId: string, section: string, body: { value?: unknown; items?: unknown }): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-import/draft`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ runId, section, ...body }),
  })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to store the ${section} (${String(res.status)})`)
}

// Checks that the draft is complete and stores it as the run's proposal; the run is then ended as ready by the caller.
export async function finishDraftPieces(runId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-import/draft/finish`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ runId }),
  })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to finish the draft (${String(res.status)})`)
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
  // generating while the AI analyses the CV, ready once its proposal is stored, failed when it produced none (step 155). A
  // generating run holds no proposal yet (aiProposal is null); every run from before step 155 is ready.
  status: 'generating' | 'ready' | 'failed'
  error?: string
  // "code" for a run whose steps the page does itself, asking a model small questions (plan 209); absent for the AI working with tools.
  driver?: string
  // What a generating run has stored so far; absent when nothing was.
  progress?: CVImportProgress
}

export interface CVImportProgress {
  profile: boolean
  persona: boolean
  personalDetails: boolean
  skills: number
  experience: number
  education: number
}

export async function fetchCVImportRuns(): Promise<CVImportRun[]> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs`, {
    method: 'GET',
    credentials: 'include',
  })
  const body = await jsonOrThrow<{ runs: CVImportRun[] }>(res, 'list cv import runs')
  return body.runs
}

// One import run by id (plan 166); null when there is no such run (it was deleted, or the address is wrong).
export async function fetchCVImportRun(id: string): Promise<CVImportRun | null> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs?id=${encodeURIComponent(id)}`, {
    method: 'GET',
    credentials: 'include',
  })
  if (res.status === 404 || res.status === 400) return null
  const body = await jsonOrThrow<{ run: CVImportRun }>(res, 'read cv import run')
  return body.run
}

// Without aiProposal the run starts "generating" (the AI hands its proposal over with suggest_cv_import and finishCVImportRun ends
// the run); with one it is recorded as a completed analysis, as before.
export async function createCVImportRun(params: {
  fileId: string
  originalFilename: string
  conversationId: string
  aiProposal?: unknown
  driver?: 'code'
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

// Ends a generating run once the AI's turn is over: "ready" (the server answers with the run, proposal included, and refuses when the
// AI never handed one over) or "failed" with the reason. A refusal throws an Error with the server's own message.
export async function finishCVImportRun(
  id: string,
  outcome: { status: 'ready' } | { status: 'failed'; error: string },
): Promise<CVImportRun> {
  const res = await fetch(`${PROXY_BASE}/cv-import/runs`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, ...outcome }),
  })
  if (!res.ok) {
    const text = (await res.text()).trim()
    throw new Error(text !== '' ? text : `failed to finish cv import run (${res.status})`)
  }
  const body = (await res.json()) as { run: CVImportRun }
  return body.run
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

// A persona a CV is attached to (step 114); builtFrom marks the one its data came from, which
// cannot be detached.
export interface CvPersona {
  id: string
  name: string
  builtFrom: boolean
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
  // The photo the CV was made with, one of the profile's photos; absent when none was chosen
  // (the profile's default, or no photo). See plan/ai/tools/career/step-111-many-photos-per-profile.md.
  photoMediaFileId?: string
  // Every persona the CV is attached to: the built-from one first, then the others by name.
  personas: CvPersona[]
  // Filled only by fetchAllCvDocuments (the "Select existing" modal's list).
  personaName?: string
  createdAt: string
  updatedAt?: string
}

// Attaches a CV to another persona of the same profile (step 114). Idempotent.
export async function attachCvToPersona(cvDocumentId: string, personaId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/personas`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ cvDocumentId, personaId }),
  })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to attach the CV (${res.status})`)
}

// Detaches a CV from a persona it was attached to; the CV itself stays. The persona it was built
// from cannot be detached.
export async function detachCvFromPersona(cvDocumentId: string, personaId: string): Promise<void> {
  const query = `cvDocumentId=${encodeURIComponent(cvDocumentId)}&personaId=${encodeURIComponent(personaId)}`
  const res = await fetch(`${PROXY_BASE}/cv-documents/personas?${query}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to detach the CV (${res.status})`)
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

// Answers "does this persona's own profile have a photo uploaded, and which?" — the profile's
// photos, the default first. A separate, deliberately tiny call rather than folding it into
// fetchCvPersonaDefaults: this is also called from the "edit an existing CV" load path, which must
// NOT re-fetch/overwrite that document's own already-loaded CvData the way persona-defaults would.
export async function fetchCvPhotoStatus(personaId: string): Promise<{ hasPhoto: boolean; photos: CvPhotoInfo[] }> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/photo-status?personaId=${encodeURIComponent(personaId)}`, {
    credentials: 'include',
  })
  return jsonOrThrow<{ hasPhoto: boolean; photos: CvPhotoInfo[] }>(res, 'check profile photo')
}

export interface CvPhotoInfo {
  mediaFileId: string
  isDefault: boolean
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
  photoMediaFileId?: string,
): Promise<CvDocument> {
  const res = await fetch(`${PROXY_BASE}/cv-documents`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ personaId, templateId, title, data, attachPhoto, photoMediaFileId }),
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
  photoMediaFileId?: string,
): Promise<{ previewUrl: string }> {
  const res = await fetch(`${PROXY_BASE}/cv-documents/preview`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ personaId, templateId, data, attachPhoto, photoMediaFileId }),
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
  if (!res.ok && res.status !== 204) {
    // 502 is the server saying, in plain words, that the CV's PDF could not be deleted and the CV was kept: that text is the message
    // (an empty body is the proxy's own 502, which keeps the generic text). Every other failure keeps its generic text.
    if (res.status === 502) {
      const text = (await res.text().catch(() => '')).trim()
      if (text !== '') throw new Error(text)
    }
    throw new Error(`failed to delete cv document (${res.status})`)
  }
}

// Every persona's CV documents, newest first (at most 200), each with its
// persona's name — what the Jobs page's "Select existing" modal picks from.
export async function fetchAllCvDocuments(): Promise<CvDocument[]> {
  const res = await fetch(`${PROXY_BASE}/cv-documents?all=1`, { credentials: 'include' })
  const body = await jsonOrThrow<{ cvDocuments: CvDocument[] }>(res, 'load cv documents')
  return body.cvDocuments
}

// --- create a CV with AI (step 115) ---
// A run is one AI attempt to create a CV for a persona: the hidden conversation, the optional note
// on what the CV is for, how it went, and the CV the AI saved.

export type CvAiRunStatus = 'generating' | 'ready' | 'failed'

export interface CvAiRun {
  id: string
  personaId: string
  conversationId: string
  focus: string
  status: CvAiRunStatus
  // The CV the AI saved; missing until it did (and after that CV was deleted).
  cvDocumentId?: string
  error?: string
  createdAt: string
  finishedAt?: string
}

export async function fetchCvAiRuns(personaId: string): Promise<CvAiRun[]> {
  const res = await fetch(`${PROXY_BASE}/cv-ai-runs?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ runs: CvAiRun[] }>(res, 'load CV AI runs')
  return data.runs
}

// Records the start of a run, before the first message is sent, so a reload can resume it. A 409
// says another run is already working on the persona.
export async function createCvAiRun(personaId: string, conversationId: string, focus: string): Promise<string> {
  const res = await fetch(`${PROXY_BASE}/cv-ai-runs`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ personaId, conversationId, focus }),
  })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to record the CV AI run (${res.status})`)
  return ((await res.json()) as { id: string }).id
}

// Ends a generating run: ready (the server refuses it, 400, when the AI saved no CV), or failed
// with a reason.
export async function finishCvAiRun(id: string, outcome: { status: 'ready' } | { status: 'failed'; error: string }): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-ai-runs`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, ...outcome }),
  })
  if (!res.ok) throw new Error((await res.text()).trim() || `failed to finish the CV AI run (${res.status})`)
}

export async function deleteCvAiRun(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/cv-ai-runs?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw new Error(`failed to remove the run (${res.status})`)
}

// --- job board (step 126) ---

// One job as a card in a column of a profile's board.
export interface JobBoardCard {
  jobId: string
  title: string
  company: string
  location: string
  updatedAt: string
  // The job's match score when that match was made for this board's profile (a job has one match result);
  // absent otherwise (step 129).
  matchScore?: number
  // How well each of this board's profile's personas fits the job, best first (step 141); absent when the job has no scores.
  personas?: JobBoardPersonaScore[]
  // What the protocol of this job's application says (step 176); absent when it has no entries.
  protocol?: JobBoardProtocol
}

// The protocol of one application in short: how many entries, how many of them are still to come (planned), and the newest one that
// has happened (a plain date for a day without a time, else an ISO UTC date and time) with its direction; both absent when nothing has
// happened yet.
export interface JobBoardProtocol {
  count: number
  planned: number
  lastAt?: string
  lastDirection?: 'sent' | 'received' | 'note'
}

// One persona's score for a job on a board; best marks the persona that fits the job best.
export interface JobBoardPersonaScore {
  personaId: string
  personaName: string
  score: number
  best?: boolean
}

// How the cards inside a column are ordered: most recently moved first (the default) or best match first.
export type BoardSort = 'moved' | 'match'

// A column is a status: one of the four fixed ones or one of the profile's own ('custom').
export type JobStatusKind = 'not_reviewed' | 'declined' | 'stopped' | 'applied' | 'custom'

export interface JobBoardColumn {
  id: string
  name: string
  kind: JobStatusKind
  jobs: JobBoardCard[]
}

export interface JobBoard {
  profileId: string
  columns: JobBoardColumn[]
}

export async function fetchJobBoard(profileId: string, sort: BoardSort = 'moved'): Promise<JobBoard> {
  const res = await fetch(`${PROXY_BASE}/job-board?profileId=${encodeURIComponent(profileId)}${sort === 'match' ? '&sort=match' : ''}`, {
    credentials: 'include',
  })
  return jsonOrThrow<JobBoard>(res, 'load the job board')
}

// One application of a profile in the overview (step 176): a job in its Applied status and/or with entries in its protocol.
// status is absent when the job is not on the board; protocol when there are no entries; activityAt (an ISO UTC time) when nothing
// has happened yet; waitingForReply is true when the last thing that happened was a message the profile sent.
export interface JobApplication {
  jobId: string
  title: string
  company: string
  status?: { id: string; name: string; kind: JobStatusKind }
  protocol?: JobBoardProtocol
  activityAt?: string
  waitingForReply: boolean
}

// A profile's applications, the one with the most recent activity first.
export async function fetchJobApplications(profileId: string): Promise<JobApplication[]> {
  const res = await fetch(`${PROXY_BASE}/job-applications?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await jsonOrThrow<{ applications: JobApplication[] }>(res, 'load the applications')).applications
}

// Puts a job on a profile's board in a status: adds it when it is not on the board yet, moves it otherwise.
export async function setJobBoardEntry(profileId: string, jobId: string, statusId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/job-board-entries`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ profileId, jobId, statusId }),
  })
  if (!res.ok) throw new Error(`failed to move the job (${res.status})`)
}

// Takes a job off a profile's board.
export async function removeJobBoardEntry(profileId: string, jobId: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/job-board-entries?profileId=${encodeURIComponent(profileId)}&jobId=${encodeURIComponent(jobId)}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (!res.ok) throw new Error(`failed to remove the job from the board (${res.status})`)
}

// The statuses calls answer a refused name with the reason as plain text ("a status with this name already
// exists"); that text is what the board shows.
async function statusFailure(res: Response, action: string): Promise<Error> {
  const text = (await res.text()).trim()
  return new Error(
    res.status === 400 || res.status === 409 ? text || `failed to ${action} (${res.status})` : `failed to ${action} (${res.status})`,
  )
}

// Adds a status of the profile's own (a new column on its board).
export async function createJobStatus(profileId: string, name: string): Promise<{ id: string; name: string }> {
  const res = await fetch(`${PROXY_BASE}/job-statuses`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, name }),
  })
  if (!res.ok) throw await statusFailure(res, 'add the status')
  return res.json() as Promise<{ id: string; name: string }>
}

export async function renameJobStatus(id: string, name: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/job-statuses`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, name }) })
  if (!res.ok) throw await statusFailure(res, 'rename the status')
}

// Deletes a status of the profile's own; the jobs in it go back to Not Reviewed.
export async function deleteJobStatus(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/job-statuses?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw await statusFailure(res, 'delete the status')
}

// One profile's board that has a job, with the job's status on it.
export interface JobBoardUsageEntry {
  profileId: string
  firstName: string
  lastName: string
  statusId: string
  statusName: string
  kind: JobStatusKind
}

// The job's status on every profile's board that has it.
export async function fetchJobBoardUsage(jobId: string): Promise<JobBoardUsageEntry[]> {
  const res = await fetch(`${PROXY_BASE}/job-board-usage?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ profiles: JobBoardUsageEntry[] }>(res, 'load the job’s boards')
  return data.profiles
}

// A status as the job page offers it: one of the four fixed ones (no profileId) or a profile's own.
export interface JobStatusInfo {
  id: string
  name: string
  kind: JobStatusKind
  profileId?: string
}

// Every status: the fixed ones and every profile's own.
export async function fetchJobStatuses(): Promise<JobStatusInfo[]> {
  const res = await fetch(`${PROXY_BASE}/job-statuses`, { credentials: 'include' })
  const data = await jsonOrThrow<{ statuses: JobStatusInfo[] }>(res, 'load the statuses')
  return data.statuses
}

// --- auto-add rules of a profile (step 140) ---

// "A new job that matches this profile at least minPercent percent is added to its board with this status."
export interface ProfileMatchRule {
  id: string
  profileId: string
  minPercent: number
  statusId: string
  statusName: string
  // The rule's place in the profile's list, 0 first; the lists come in this order (step 143).
  position: number
}

// The rules calls answer a refused value with the reason as plain text ("this profile already has a rule for this
// percentage"); that text is what the settings page shows.
async function ruleFailure(res: Response, action: string): Promise<Error> {
  const text = (await res.text()).trim()
  return new Error(
    res.status === 400 || res.status === 409 ? text || `failed to ${action} (${res.status})` : `failed to ${action} (${res.status})`,
  )
}

// One profile's rules, highest percentage first.
export async function fetchProfileMatchRules(profileId: string): Promise<ProfileMatchRule[]> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ rules: ProfileMatchRule[] }>(res, 'load the rules')
  return data.rules
}

export async function createProfileMatchRule(profileId: string, minPercent: number, statusId: string): Promise<ProfileMatchRule> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, minPercent, statusId }),
  })
  if (!res.ok) throw await ruleFailure(res, 'add the rule')
  return res.json() as Promise<ProfileMatchRule>
}

export async function updateProfileMatchRule(id: string, minPercent: number, statusId: string): Promise<ProfileMatchRule> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, minPercent, statusId }),
  })
  if (!res.ok) throw await ruleFailure(res, 'change the rule')
  return res.json() as Promise<ProfileMatchRule>
}

// Sets the order of a profile's rules (the first id is tried first) and returns them in that order. The ids must be exactly the
// profile's rules; the server refuses anything else and changes nothing.
export async function reorderProfileMatchRules(profileId: string, ruleIds: string[]): Promise<ProfileMatchRule[]> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules/order`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ profileId, ruleIds }),
  })
  if (!res.ok) throw await ruleFailure(res, 'reorder the rules')
  const data = (await res.json()) as { rules: ProfileMatchRule[] }
  return data.rules
}

// What applying a profile's rules to the jobs that already exist did, or in a preview would do (step 143). Every job looked
// at is in exactly one of notJudged, below, onBoard and total (added, or to be added); statuses says where the added ones go.
export interface ApplyRulesSummary {
  jobs: number
  notJudged: number
  below: number
  onBoard: number
  removed: number
  total: number
  statuses: { statusId: string; statusName: string; count: number }[]
}

// A failed apply: the reason, and what was done before the failure when the server says (what was added stays).
export class ApplyRulesError extends Error {
  readonly summary: ApplyRulesSummary | null
  constructor(message: string, summary: ApplyRulesSummary | null) {
    super(message)
    this.name = 'ApplyRulesError'
    this.summary = summary
  }
}

// Applies the profile's rules to the existing jobs (dryRun: only counts what it would do). A card already on the board is never
// moved, and a job the person took off the board on purpose is left out unless includeRemoved is true (step 144). The refusals
// (no rules, a pass already running) come back as an error with the server's own words.
export async function applyProfileMatchRules(profileId: string, dryRun: boolean, includeRemoved = false): Promise<ApplyRulesSummary> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules/apply`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, dryRun, includeRemoved }),
  })
  if (res.ok) return (await res.json()) as ApplyRulesSummary
  if (res.status === 500) {
    const body = (await res.json().catch(() => null)) as { error?: string; summary?: ApplyRulesSummary } | null
    throw new ApplyRulesError(body?.error ?? 'failed to apply the rules (500)', body?.summary ?? null)
  }
  const text = (await res.text()).trim()
  throw new ApplyRulesError(
    res.status === 400 || res.status === 409
      ? text || `failed to apply the rules (${String(res.status)})`
      : `failed to apply the rules (${String(res.status)})`,
    null,
  )
}

// --- recrawl jobs for a fresh description (step 148) ---

// What a request to recrawl jobs answered: the batch to follow, how many jobs were queued, and the ones refused at once.
export interface RecrawlStarted {
  batchId: string
  accepted: number
  skipped: SkippedJob[]
}

// A request that queued nothing (400): the server's words and the jobs it skipped with their reasons.
export class RecrawlRefused extends Error {
  readonly skipped: SkippedJob[]
  constructor(message: string, skipped: SkippedJob[]) {
    super(message)
    this.name = 'RecrawlRefused'
    this.skipped = skipped
  }
}

// Queues a recrawl of the jobs, one event each, however many there are. The crawling happens on the server, one job after the other.
export async function requestRecrawl(jobIds: string[]): Promise<RecrawlStarted> {
  const res = await fetch(`${PROXY_BASE}/jobs/recrawl`, { method: 'POST', credentials: 'include', body: JSON.stringify({ jobIds }) })
  if (res.status === 202) return (await res.json()) as RecrawlStarted
  const text = await res.text()
  if (res.status === 400) {
    try {
      const body = JSON.parse(text) as { error?: string; skipped?: SkippedJob[] }
      throw new RecrawlRefused(body.error ?? 'none of these jobs can be recrawled', body.skipped ?? [])
    } catch (err) {
      if (err instanceof RecrawlRefused) throw err
      throw new Error(text.trim() || 'failed to start the recrawl (400)')
    }
  }
  throw new Error(`failed to start the recrawl (${String(res.status)})`)
}

// The jobs whose detail page was never crawled successfully (never tried, or the last try failed): jobIds are the ones that can be crawled
// now; the counts say why the rest cannot. Nothing here depends on a crawl that is running.
// Where a job without crawled details stands (plan 214); the first three say a crawl is under way or next, ready that nothing is scheduled, the rest
// why it cannot be crawled yet.
export type MissingDetailState = 'running' | 'queued' | 'scheduled' | 'ready' | 'noInstructions' | 'needsLogin' | 'noPortal'

export interface MissingDetails {
  jobIds: string[]
  total: number
  noPortal: number
  noInstructions: number
  needsLogin: number
  alreadyBeingCrawled: number
  // By job id, for every job without a successful detail crawl. Empty from a backend that predates it (the Jobs page then promises nothing).
  states: Record<string, MissingDetailState>
}

export async function fetchMissingDetails(): Promise<MissingDetails> {
  const res = await fetch(`${PROXY_BASE}/jobs/recrawl/missing`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`failed to look for jobs without details (${String(res.status)})`)
  const body = (await res.json()) as Partial<MissingDetails>
  return {
    jobIds: body.jobIds ?? [],
    total: body.total ?? 0,
    noPortal: body.noPortal ?? 0,
    noInstructions: body.noInstructions ?? 0,
    needsLogin: body.needsLogin ?? 0,
    alreadyBeingCrawled: body.alreadyBeingCrawled ?? 0,
    states: body.states ?? {},
  }
}

// What the job recrawl is doing right now, for the Crawl Monitor (plan 194): the batches with work left and the jobs being crawled at this
// moment. A recrawl has no crawl run, so the monitor's own list never showed it.
export interface RecrawlActiveBatch {
  batchId: string
  total: number
  done: number
  queued: number
  running: number
  cancelled: number
  startedAt: string
  // Stop was clicked: nothing waits any more and some crawls are still being finished.
  stopping: boolean
}

export interface RecrawlActiveWorker {
  jobId: string
  jobTitle: string
  portalId?: string
  portalName?: string
  portalLinkId?: string
  portalLinkTitle?: string
  url: string
  // The crawler that took the job: 0 is the main crawler, 1 to 4 the helpers; -1 when the row does not say.
  slot: number
  startedAt: string
  // The Browser tool's own words; absent while it has said nothing (it says nothing while it waits for the platform's turn).
  phase?: string
  message?: string
}

export interface RecrawlActive {
  batches: RecrawlActiveBatch[]
  workers: RecrawlActiveWorker[]
}

export async function fetchRecrawlActive(): Promise<RecrawlActive> {
  const res = await fetch(`${PROXY_BASE}/jobs/recrawl/active`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw new Error(`failed to load the job recrawl (${String(res.status)})`)
  const body = (await res.json()) as Partial<RecrawlActive>
  return { batches: body.batches ?? [], workers: body.workers ?? [] }
}

// The progress of a batch: the numbers for all of it and the first `limit` jobs (1 is enough for the numbers). null when the server no
// longer knows the batch (404).
export async function fetchRecrawlStatus(batchId: string, limit: number): Promise<BatchStatus | null> {
  const res = await fetch(`${PROXY_BASE}/jobs/recrawl/status?batchId=${encodeURIComponent(batchId)}&limit=${String(limit)}`, {
    credentials: 'include',
  })
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`failed to read the recrawl (${String(res.status)})`)
  return (await res.json()) as BatchStatus
}

// Stops the batch's jobs that have not started; a job being crawled finishes its page. Returns how many were cancelled.
export async function cancelRecrawl(batchId: string): Promise<number> {
  const res = await fetch(`${PROXY_BASE}/jobs/recrawl/cancel`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ batchId }),
  })
  if (res.status === 404) return 0
  if (!res.ok) throw new Error(`failed to stop the recrawl (${String(res.status)})`)
  return ((await res.json()) as { cancelled: number }).cancelled
}

export async function deleteProfileMatchRule(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profile-match-rules?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw await ruleFailure(res, 'delete the rule')
}

// --- the personas' scores for a job (step 141) ---

// How well one persona fits a job, with who it is.
export interface JobPersonaMatch {
  personaId: string
  personaName: string
  profileId: string
  profileName: string
  score: number
}

// Every persona of every profile with its score for the job, best first; best is the best persona's id ('' when none).
export interface JobPersonaMatches {
  matches: JobPersonaMatch[]
  best: string
}

export async function fetchJobPersonaMatches(jobId: string): Promise<JobPersonaMatches> {
  const res = await fetch(`${PROXY_BASE}/job-persona-matches?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  return jsonOrThrow<JobPersonaMatches>(res, 'load the persona matches')
}

// Scores the job for every persona now and answers like the read.
export async function recalculateJobPersonaMatches(jobId: string): Promise<JobPersonaMatches> {
  const res = await fetch(`${PROXY_BASE}/job-persona-matches/recalculate`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ jobId }),
  })
  return jsonOrThrow<JobPersonaMatches>(res, 'score the job for the personas')
}

// --- manual (no AI) job match (step 129) ---

// One part of a manual score: its weight, the points it gave, or why it could not be judged.
export interface MatchComponent {
  key: string
  label: string
  weight: number
  points: number
  evaluable: boolean
  note: string
}

// A requirement of tier 2 or 3 that nothing in the persona covers.
export interface MatchMissing {
  term: string
  tier: number
}

// What the manual engine answers for one job and one profile (and what the job's matchBreakdown holds).
export interface ManualMatchResult {
  score: number
  personaId: string
  personaName: string
  matchedSkills: string[]
  mode: 'requirements' | 'text'
  coverage: number
  lowConfidence: boolean
  // 1, or less when the job hardly resembles the persona's field and the score was scaled down (absent in results
  // stored before that rule).
  relevanceScale?: number
  // 1, or less when the persona's explicit remote choice rules the job out (remote only vs an on-site job, on-site only
  // vs a remote job) and the score was scaled down (absent in results stored before that rule).
  dealbreakerScale?: number
  components: MatchComponent[]
  missing: MatchMissing[]
  gaps: string[]
  engineVersion: string
}

// A refused manual match: the reasons a person can act on ("this persona has no skills ...").
export class ManualMatchError extends Error {
  readonly reasons: string[]
  constructor(message: string, reasons: string[]) {
    super(message)
    this.reasons = reasons
  }
}

// Scores a job against a profile's personas with the manual engine and stores the best result as the job's match.
// No AI is involved. A 422 (not enough data, no persona) throws a ManualMatchError with the reasons.
export async function runManualMatch(jobId: string, profileId: string): Promise<ManualMatchResult> {
  const res = await fetch(`${PROXY_BASE}/jobs/match/run`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ jobId, profileId }),
  })
  if (res.status === 422) {
    const body = (await res.json().catch(() => ({}))) as { error?: string; reasons?: string[] }
    throw new ManualMatchError(body.error ?? 'not enough data to match', body.reasons ?? [])
  }
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(text || `failed to match the job (${res.status})`)
  }
  return res.json() as Promise<ManualMatchResult>
}

// --- job requirements (step 129) ---

// One thing a job asks for, with its tier (3 must-have, 2 important, 1 nice to have). Written by hand on the job
// page; the manual job match scores a persona against them.
export interface JobRequirement {
  id: string
  term: string
  tier: number
}

export interface JobRequirementInput {
  term: string
  tier: number
}

export async function fetchJobRequirements(jobId: string): Promise<JobRequirement[]> {
  const res = await fetch(`${PROXY_BASE}/job-requirements?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ requirements: JobRequirement[] }>(res, 'load the requirements')
  return data.requirements
}

// Replaces the job's whole list (an empty list clears it). A refused list comes back as its reason in plain text.
export async function saveJobRequirements(jobId: string, requirements: JobRequirementInput[]): Promise<JobRequirement[]> {
  const res = await fetch(`${PROXY_BASE}/job-requirements`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ jobId, requirements }),
  })
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(res.status === 400 && text ? text : `failed to save the requirements (${res.status})`)
  }
  const data = (await res.json()) as { requirements: JobRequirement[] }
  return data.requirements
}

// The job's most distinctive words as candidate requirements (a word count, never an AI call).
export async function fetchRequirementSuggestions(jobId: string): Promise<string[]> {
  const res = await fetch(`${PROXY_BASE}/job-requirements/suggest?jobId=${encodeURIComponent(jobId)}`, { credentials: 'include' })
  const data = await jsonOrThrow<{ suggestions: string[] }>(res, 'load the suggestions')
  return data.suggestions
}

// --- the profile's library of skills and experience (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md) ---

// A skill of the profile's library: global to the profile, and used by some of its personas.
export interface ProfileSkill {
  id: string
  profileId: string
  skill: string
  createdAt: string
  // How many personas use it.
  usedBy: number
}

// An experience of the profile's library, with how many personas use it.
export interface ProfileExperience {
  id: string
  profileId: string
  company: string
  title: string
  startDate?: string
  endDate?: string
  description?: string
  createdAt: string
  usedBy: number
}

export interface ProfileExperienceInput {
  company: string
  title: string
  startDate: string
  endDate: string
  description: string
}

// Unlike jsonOrThrow the answer's own text is the message when there is one: a refusal of the library ("the profile already has
// this skill", "a skill needs a name") says what is wrong.
async function libraryJson<T>(res: Response, action: string): Promise<T> {
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(text !== '' ? text : `failed to ${action} (${String(res.status)})`)
  }
  return res.json() as Promise<T>
}

export async function fetchProfileSkills(profileId: string): Promise<ProfileSkill[]> {
  const res = await fetch(`${PROXY_BASE}/profile-skills?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ skills: ProfileSkill[] }>(res, 'load the skills')).skills
}

export async function addProfileSkill(profileId: string, skill: string): Promise<ProfileSkill[]> {
  const res = await fetch(`${PROXY_BASE}/profile-skills`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, skill }),
  })
  return (await libraryJson<{ skills: ProfileSkill[] }>(res, 'add the skill')).skills
}

export async function renameProfileSkill(id: string, skill: string): Promise<ProfileSkill[]> {
  const res = await fetch(`${PROXY_BASE}/profile-skills`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, skill }) })
  return (await libraryJson<{ skills: ProfileSkill[] }>(res, 'rename the skill')).skills
}

export async function removeProfileSkill(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profile-skills?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the skill')
}

export async function fetchProfileExperience(profileId: string): Promise<ProfileExperience[]> {
  const res = await fetch(`${PROXY_BASE}/profile-experience?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ experience: ProfileExperience[] }>(res, 'load the experience')).experience
}

export async function addProfileExperience(profileId: string, entry: ProfileExperienceInput): Promise<ProfileExperience[]> {
  const res = await fetch(`${PROXY_BASE}/profile-experience`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, ...entry }),
  })
  return (await libraryJson<{ experience: ProfileExperience[] }>(res, 'add the experience')).experience
}

export async function updateProfileExperience(id: string, entry: Partial<ProfileExperienceInput>): Promise<ProfileExperience[]> {
  const res = await fetch(`${PROXY_BASE}/profile-experience`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...entry }),
  })
  return (await libraryJson<{ experience: ProfileExperience[] }>(res, 'update the experience')).experience
}

export async function removeProfileExperience(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/profile-experience?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the experience')
}

// --- what a persona uses of its profile's library (plan 173, step 4) ---

// The persona's skills page: which library skills it uses (ids), the library to choose from, and its own custom skills.
export interface PersonaSkillsView {
  used: string[]
  library: ProfileSkill[]
  own: { id: string; skill: string }[]
}

// One used library experience and the persona's own wording of it; a missing field follows the profile's text.
export interface ExperienceUse {
  id: string
  titleOverride?: string
  descriptionOverride?: string
}

export interface PersonaExperienceView {
  used: ExperienceUse[]
  library: ProfileExperience[]
  own: CareerExperience[]
}

export async function fetchPersonaSkillsView(personaId: string): Promise<PersonaSkillsView> {
  const res = await fetch(`${PROXY_BASE}/persona-skills?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  return libraryJson<PersonaSkillsView>(res, 'load the persona’s skills')
}

// Replaces the persona's whole choice of library skills with exactly these ids.
export async function setPersonaSkillIds(personaId: string, skillIds: string[]): Promise<PersonaSkillsView> {
  const res = await fetch(`${PROXY_BASE}/persona-skills`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ personaId, skillIds }),
  })
  return libraryJson<PersonaSkillsView>(res, 'save the persona’s skills')
}

export async function fetchPersonaExperienceView(personaId: string): Promise<PersonaExperienceView> {
  const res = await fetch(`${PROXY_BASE}/persona-experience?personaId=${encodeURIComponent(personaId)}`, { credentials: 'include' })
  const view = await libraryJson<PersonaExperienceView>(res, 'load the persona’s experience')
  return { ...view, own: normalizeExperience(view.own) }
}

// Replaces the persona's whole choice of library experiences, with their wording, by exactly this list.
export async function setPersonaExperienceUse(personaId: string, experience: ExperienceUse[]): Promise<PersonaExperienceView> {
  const res = await fetch(`${PROXY_BASE}/persona-experience`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ personaId, experience }),
  })
  const view = await libraryJson<PersonaExperienceView>(res, 'save the persona’s experience')
  return { ...view, own: normalizeExperience(view.own) }
}

// --- the profile's education (plan 173, step 6 and 7) ---

export interface Education {
  id: string
  profileId: string
  institution: string
  degree: string
  fieldOfStudy: string
  startDate: string
  endDate: string
  description: string
  createdAt: string
}

export interface EducationInput {
  institution: string
  degree: string
  fieldOfStudy: string
  startDate: string
  endDate: string
  description: string
}

export async function fetchEducation(profileId: string): Promise<Education[]> {
  const res = await fetch(`${PROXY_BASE}/education?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ education: Education[] }>(res, 'load the education')).education
}

export async function addEducation(profileId: string, entry: EducationInput): Promise<Education[]> {
  const res = await fetch(`${PROXY_BASE}/education`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, ...entry }),
  })
  return (await libraryJson<{ education: Education[] }>(res, 'add the education')).education
}

export async function updateEducation(id: string, entry: Partial<EducationInput>): Promise<Education[]> {
  const res = await fetch(`${PROXY_BASE}/education`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, ...entry }) })
  return (await libraryJson<{ education: Education[] }>(res, 'update the education')).education
}

export async function removeEducation(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/education?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the education')
}

// --- the profile's languages, courses and hobbies (plan 231) ---

export interface Language {
  id: string
  profileId: string
  language: string
  // '' (not given), 'native', or a CEFR level: 'c2', 'c1', 'b2', 'b1', 'a2', 'a1'.
  level: string
  canSpeak: boolean
  canWrite: boolean
  createdAt: string
}

export interface LanguageInput {
  language: string
  level: string
  canSpeak: boolean
  canWrite: boolean
}

export async function fetchLanguages(profileId: string): Promise<Language[]> {
  const res = await fetch(`${PROXY_BASE}/languages?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ languages: Language[] }>(res, 'load the languages')).languages
}

export async function addLanguage(profileId: string, entry: LanguageInput): Promise<Language[]> {
  const res = await fetch(`${PROXY_BASE}/languages`, { method: 'POST', credentials: 'include', body: JSON.stringify({ profileId, ...entry }) })
  return (await libraryJson<{ languages: Language[] }>(res, 'add the language')).languages
}

export async function updateLanguage(id: string, entry: Partial<LanguageInput>): Promise<Language[]> {
  const res = await fetch(`${PROXY_BASE}/languages`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, ...entry }) })
  return (await libraryJson<{ languages: Language[] }>(res, 'update the language')).languages
}

export async function removeLanguage(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/languages?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the language')
}

export interface Course {
  id: string
  profileId: string
  title: string
  provider: string
  completedDate: string
  credentialUrl: string
  description: string
  createdAt: string
}

export interface CourseInput {
  title: string
  provider: string
  completedDate: string
  credentialUrl: string
  description: string
}

export async function fetchCourses(profileId: string): Promise<Course[]> {
  const res = await fetch(`${PROXY_BASE}/courses?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ courses: Course[] }>(res, 'load the courses')).courses
}

export async function addCourse(profileId: string, entry: CourseInput): Promise<Course[]> {
  const res = await fetch(`${PROXY_BASE}/courses`, { method: 'POST', credentials: 'include', body: JSON.stringify({ profileId, ...entry }) })
  return (await libraryJson<{ courses: Course[] }>(res, 'add the course')).courses
}

export async function updateCourse(id: string, entry: Partial<CourseInput>): Promise<Course[]> {
  const res = await fetch(`${PROXY_BASE}/courses`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, ...entry }) })
  return (await libraryJson<{ courses: Course[] }>(res, 'update the course')).courses
}

export async function removeCourse(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/courses?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the course')
}

export interface Hobby {
  id: string
  profileId: string
  name: string
  description: string
  createdAt: string
}

export interface HobbyInput {
  name: string
  description: string
}

export async function fetchHobbies(profileId: string): Promise<Hobby[]> {
  const res = await fetch(`${PROXY_BASE}/hobbies?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return (await libraryJson<{ hobbies: Hobby[] }>(res, 'load the hobbies')).hobbies
}

export async function addHobby(profileId: string, entry: HobbyInput): Promise<Hobby[]> {
  const res = await fetch(`${PROXY_BASE}/hobbies`, { method: 'POST', credentials: 'include', body: JSON.stringify({ profileId, ...entry }) })
  return (await libraryJson<{ hobbies: Hobby[] }>(res, 'add the hobby')).hobbies
}

export async function updateHobby(id: string, entry: Partial<HobbyInput>): Promise<Hobby[]> {
  const res = await fetch(`${PROXY_BASE}/hobbies`, { method: 'PUT', credentials: 'include', body: JSON.stringify({ id, ...entry }) })
  return (await libraryJson<{ hobbies: Hobby[] }>(res, 'update the hobby')).hobbies
}

export async function removeHobby(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/hobbies?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 204) await libraryJson<unknown>(res, 'remove the hobby')
}

// --- the summary and the goals of a profile (plan 174, step 3 and 4) ---

// What every persona of the profile follows unless it has its own. minSalary 0 is "no minimum".
export interface ProfileDetails {
  profileId: string
  summary: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
  remotePreference: '' | 'onsite' | 'open' | 'remote_only'
}

export type ProfileDetailsPatch = Partial<Omit<ProfileDetails, 'profileId'>>

export async function fetchProfileDetails(profileId: string): Promise<ProfileDetails> {
  const res = await fetch(`${PROXY_BASE}/profile-details?profileId=${encodeURIComponent(profileId)}`, { credentials: 'include' })
  return libraryJson<ProfileDetails>(res, 'load the profile’s summary and goals')
}

export async function updateProfileDetails(profileId: string, patch: ProfileDetailsPatch): Promise<ProfileDetails> {
  const res = await fetch(`${PROXY_BASE}/profile-details`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ profileId, ...patch }),
  })
  return libraryJson<ProfileDetails>(res, 'save the profile’s summary and goals')
}

// --- the application protocol (plan/ai/tools/career/step-176-application-protocol.md, step 3) ---

export type ApplicationDirection = 'sent' | 'received' | 'note'
export type ApplicationChannel = 'email' | 'platform' | 'sms' | 'whatsapp' | 'phone' | 'video' | 'in_person' | 'letter' | 'other'

// One entry of the log. occurredAt is a plain date ("2026-11-05") when hasTime is false, else an ISO UTC date and time.
export interface ApplicationMessage {
  id: string
  profileId: string
  jobId: string
  direction: ApplicationDirection
  channel: ApplicationChannel
  channelLabel: string
  occurredAt: string
  hasTime: boolean
  subject: string
  body: string
  recruiterId: string
  counterpartName: string
  counterpartContact: string
  cvDocumentId: string
  externalUrl: string
  createdAt: string
  updatedAt?: string
}

// What is sent to write an entry; the server checks all of it again. An empty text clears a field on an update.
export interface ApplicationMessageInput {
  direction: ApplicationDirection
  channel: ApplicationChannel
  channelLabel: string
  occurredAt: string
  subject: string
  body: string
  recruiterId: string
  counterpartName: string
  counterpartContact: string
  cvDocumentId: string
  externalUrl: string
}

export async function fetchApplicationMessages(profileId: string, jobId: string): Promise<ApplicationMessage[]> {
  const res = await fetch(
    `${PROXY_BASE}/application-messages?profileId=${encodeURIComponent(profileId)}&jobId=${encodeURIComponent(jobId)}`,
    { credentials: 'include' },
  )
  return (await libraryJson<{ messages: ApplicationMessage[] }>(res, 'load the protocol')).messages
}

// Every write answers with the pair's list, oldest first.
export async function addApplicationMessage(
  profileId: string,
  jobId: string,
  entry: ApplicationMessageInput,
): Promise<ApplicationMessage[]> {
  const res = await fetch(`${PROXY_BASE}/application-messages`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ profileId, jobId, ...entry }),
  })
  return (await libraryJson<{ messages: ApplicationMessage[] }>(res, 'add the message')).messages
}

export async function updateApplicationMessage(id: string, entry: ApplicationMessageInput): Promise<ApplicationMessage[]> {
  const res = await fetch(`${PROXY_BASE}/application-messages`, {
    method: 'PUT',
    credentials: 'include',
    body: JSON.stringify({ id, ...entry }),
  })
  return (await libraryJson<{ messages: ApplicationMessage[] }>(res, 'save the message')).messages
}

export async function deleteApplicationMessage(id: string): Promise<void> {
  const res = await fetch(`${PROXY_BASE}/application-messages?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw new Error((await res.text().catch(() => '')).trim() || `failed to delete the message (${String(res.status)})`)
}
