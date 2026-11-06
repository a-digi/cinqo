// The Katana engine's own call for "Start over" (plan 192, D14); the shared api.ts is not touched.
import type { StartFreshResult } from '../../../api'

export type StartOverScope = 'all' | 'job_details'

const PROXY_BASE = '/api/v1/tools/career/proxy'

// The server answers 409 while a generation or a crawl is running for the link; its text is the error.
export async function katanaStartOver(portalLinkId: string, scope: StartOverScope): Promise<StartFreshResult> {
  const res = await fetch(`${PROXY_BASE}/portal-links/katana/start-over`, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ portalLinkId, scope }),
  })
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim()
    throw new Error(res.status === 409 && text !== '' ? text : `failed to start over (${res.status})`)
  }
  const data = (await res.json()) as Partial<StartFreshResult>
  return { conversationIds: data.conversationIds ?? [], attemptsRemoved: data.attemptsRemoved ?? 0 }
}

export const START_OVER_OPTIONS: readonly { scope: StartOverScope; label: string; detail: string }[] = [
  {
    scope: 'all',
    label: 'Job list and job details',
    detail: "Deletes this link's job list rule and its job detail picks. The AI can then start from the very beginning.",
  },
  {
    scope: 'job_details',
    label: 'Job details only',
    detail: 'Keeps the job list rule and deletes only the job detail picks.',
  },
]

export const START_OVER_KEEPS = 'Crawled jobs are never deleted. The AI conversations of the cleared attempts are deleted.'
