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
  source: string
  reason: string
}

export interface FieldChoice {
  label: PickField
  selector: string
  attribute?: string
}

export interface Choices {
  fields: FieldChoice[]
  descriptionSelector?: string
}

export interface CurrentInstructions {
  yaml: string
  choices?: Choices
  // What the stored instructions hold that this screen cannot show (saving the screen's choices would drop it).
  lossy: string[]
}

export interface PickCandidates {
  url: string
  finalUrl: string
  title: string
  samples: PickSample[]
  ranked: Record<PickField, Ranked[]>
  structuredHints: StructuredHint[]
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

// Load a sample page and the suggestions for it: a saved job of the link (jobId), an address on its site (url), or neither (the oldest
// saved job).
export function loadPickCandidates(
  portalLinkId: string,
  sample: { jobId?: string; url?: string },
  signal?: AbortSignal,
): Promise<PickCandidates> {
  return post<PickCandidates>('candidates', { portalLinkId, ...sample }, 'load the page', signal)
}

// The instruction text the choices make, or why they are not valid yet. Pure on the backend: nothing is loaded or stored.
export function previewPickChoices(portalLinkId: string, choices: Choices, signal?: AbortSignal): Promise<PickPreview> {
  return post<PickPreview>('preview', { portalLinkId, choices }, 'preview the instructions', signal)
}
