// jobDetailPick.ts (plan 186, step 6) — the calls of the "pick the data of a job page" screen and the shapes they answer with. The
// backend decides and enforces everything (the link must not need a login, the page must be a saved job of the link or on its own site);
// this file only asks and applies the answers.

const BASE = '/api/v1/tools/career/proxy/portal-links/job-detail'

export type PickField = 'title' | 'company' | 'location' | 'postedAt' | 'description'

// A place on the sample page a field could be read from, with its score (higher is better) and the reasons for it.
export interface Ranked {
  selector: string
  unique: boolean
  matches: number
  attribute: string
  value: string
  chars: number
  tag: string
  reason: string
  inMain: boolean
  placedIn: string
  order: number
  source: string
  shareOfPage?: number
  byAttr?: boolean
  headings?: number
  listItems?: number
  paragraphs?: number
  score: number
  why: string[]
}

export interface PageElement {
  selector: string
  unique: boolean
  tag: string
  text: string
  chars: number
}

export interface PickSample {
  jobId: string
  title: string
  url: string
  descriptionChars: number
}

export interface StructuredHint {
  field: PickField
  value: string
  // 'jsonld' or 'meta'; path is what a field reads to get this value.
  source: string
  path: string
  reason: string
}

export interface FieldChoice {
  label: PickField
  // A field is read from an element (selector, attribute) or from the page's structured data (source and path), never both.
  selector?: string
  attribute?: string
  source?: string
  path?: string
}

// A part of a description rule: a box of the page, what to leave out of it, an optional heading and where to stop.
export interface RulePart {
  // A part takes either a box (include) or the text the page states in its structured data (source and path).
  include?: string
  source?: string
  path?: string
  heading?: 'none' | 'auto' | 'selector'
  headingSelector?: string
  exclude?: string[]
  cutAfter?: string[]
}

// How a job's description is read: one or more boxes, the format and the limit (the backend fills the defaults).
export interface DescriptionRule {
  parts: RulePart[]
  format?: 'plain' | 'structured'
  limit?: number
}

export interface Choices {
  fields: FieldChoice[]
  // Exactly one of these two describes the description (the backend refuses both).
  descriptionSelector?: string
  description?: DescriptionRule
  // Let the title the page states replace the job's saved title after a detail crawl (off unless chosen; needs the title field).
  replaceTitle?: boolean
}

export interface CurrentInstructions {
  yaml: string
  choices?: Choices
  // What the stored instructions hold that this screen cannot show (saving the screen's choices would drop it).
  lossy: string[]
}

// Suggestions for a description box: what inside it looks like page furniture (to leave out) and texts it could stop at.
export interface ExcludeSuggestion {
  selector: string
  kind: string
  chars: number
  value: string
  share: number
  risky: boolean
}

export interface DescriptionHelp {
  excludes: ExcludeSuggestion[]
  cutAfter: string[]
}

// Advice about the loaded page itself (a site that served a reduced page, a page that is not the job's). Never blocks anything.
export interface PageNotice {
  code: string
  message: string
}

export interface PickCandidates {
  url: string
  finalUrl: string
  title: string
  samples: PickSample[]
  // How many saved jobs the link has (a recrawl could re-score those whose description changes).
  jobCount: number
  // Whether the installed Browser tool can read the page's structured data (JSON-LD, meta tags) as a field source.
  structuredSupported: boolean
  ranked: Record<PickField, Ranked[]>
  // Keyed by the selector of a description candidate.
  descriptionHelp: Record<string, DescriptionHelp>
  structuredHints: StructuredHint[]
  notices: PageNotice[]
  elements: PageElement[]
  pageTextChars: number
  truncated: boolean
  current: CurrentInstructions
}

export interface PickPreview {
  valid: boolean
  yaml?: string
  error?: string
}

// An error the backend explained: its message, its code ('cloudflare_challenge_unresolved', 'fetch_failed', …) and, for a site that
// stopped the page, the Browser's advice.
export class PickError extends Error {
  readonly status: number
  readonly code: string
  readonly advice: string
  constructor(message: string, status: number, code = '', advice = '') {
    super(message)
    this.name = 'PickError'
    this.status = status
    this.code = code
    this.advice = advice
  }
}

async function readError(res: Response, action: string): Promise<PickError> {
  const text = (await res.text()).trim()
  try {
    const j = JSON.parse(text) as { code?: string; message?: string; advice?: string }
    if (j.message) return new PickError(j.message, res.status, j.code ?? '', j.advice ?? '')
  } catch {
    // plain text
  }
  return new PickError(text || `failed to ${action} (${res.status})`, res.status)
}

async function post<T>(path: string, body: unknown, action: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${BASE}/${path}`, {
    method: 'POST',
    credentials: 'include',
    cache: 'no-store',
    signal,
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await readError(res, action)
  return (await res.json()) as T
}

// The lists the screen reads must be lists. A Go nil slice arrives as null (a page without structured data once crashed the whole page),
// so the answer is made safe here, once, however the backend sent it.
type RawCandidates = Omit<PickCandidates, 'samples' | 'elements' | 'structuredHints' | 'notices' | 'current'> & {
  samples: PickSample[] | null
  elements: PageElement[] | null
  structuredHints: StructuredHint[] | null
  notices: PageNotice[] | null
  current: Omit<CurrentInstructions, 'lossy' | 'choices'> & {
    lossy: string[] | null
    choices?: Omit<Choices, 'fields'> & { fields: Choices['fields'] | null }
  }
}

export function normalizeCandidates(p: RawCandidates): PickCandidates {
  const { choices, lossy, yaml } = p.current
  const current: CurrentInstructions = { yaml, lossy: lossy ?? [] }
  if (choices) current.choices = { ...choices, fields: choices.fields ?? [] }
  return {
    ...p,
    samples: p.samples ?? [],
    elements: p.elements ?? [],
    structuredHints: p.structuredHints ?? [],
    notices: p.notices ?? [],
    current,
  }
}

// Load a sample page and the suggestions for it: a saved job of the link (jobId), an address on its site (url), or neither (the oldest
// saved job).
export function loadPickCandidates(
  portalLinkId: string,
  sample: { jobId?: string; url?: string },
  signal?: AbortSignal,
): Promise<PickCandidates> {
  return post<RawCandidates>('candidates', { portalLinkId, ...sample }, 'load the page', signal).then(normalizeCandidates)
}

// The instruction text the choices make, or why they are not valid yet. Pure on the backend: nothing is loaded or stored.
export function previewPickChoices(portalLinkId: string, choices: Choices, signal?: AbortSignal): Promise<PickPreview> {
  return post<PickPreview>('preview', { portalLinkId, choices }, 'preview the instructions', signal)
}

// --- trying the choices on a sample job (the real extraction, nothing saved) ---

export interface PartReport {
  index: number
  include?: string
  matches: number
  chars: number
  overlap: boolean
  notFound: boolean
  cut: number
  removed: number
}

export interface RemovedText {
  part: number
  kind: string
  selector?: string
  chars: number
  text: string
}

export interface DescriptionReport {
  chars: number
  fullLength: number
  truncated: boolean
  limit: number
  parts?: PartReport[]
  outline?: string[]
  removed?: RemovedText[]
  removedTotal?: number
  errors?: string[]
}

export interface DiffSummary {
  unchanged: boolean
  kept: number
  added: number
  removed: number
  oldChars: number
  newChars: number
  firstAdded: string[]
  firstRemoved: string[]
}

export interface PickTestJob {
  jobId: string
  title: string
  url: string
  error?: string
  values: Record<string, string>
  postedAtClean?: string
  descriptionChars: number
  truncated?: string[]
  report?: DescriptionReport
  storedChars: number
  diff?: DiffSummary
}

export interface PickFlag {
  code: string
  severity: 'blocking' | 'warning' | 'info'
  field?: PickField
  message: string
  jobIds?: string[]
}

export interface PickTest {
  yaml: string
  jobs: PickTestJob[]
  flags: PickFlag[]
}

// Run the real extraction with the choices on up to three saved jobs of the link. Nothing is saved.
export function testPickChoices(portalLinkId: string, choices: Choices, jobIds: string[], signal?: AbortSignal): Promise<PickTest> {
  return post<PickTest>('test', { portalLinkId, choices, jobIds }, 'try the choices', signal)
}

// Store the choices as the link's job-detail instructions (the same validated function the AI's tool uses). Returns the stored text.
export function savePickChoices(portalLinkId: string, choices: Choices, signal?: AbortSignal): Promise<PickPreview> {
  return post<PickPreview>('save', { portalLinkId, choices }, 'save the instructions', signal)
}
