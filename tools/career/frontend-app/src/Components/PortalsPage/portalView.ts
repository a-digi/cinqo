// Pure numbers for the portal show page's side rail, worked out from the portal already
// loaded (no extra request). No React, so it can be script-checked.
export interface ViewLink {
  // The backend leaves an empty field out of the JSON, so "none" arrives as undefined as well as null.
  crawlInstructions?: string | null
  lastCrawledAt?: string | null
}

export interface PortalViewNumbers {
  links: number
  // Links that have crawl instructions, i.e. that can actually be crawled.
  withInstructions: number
  // Links that have never produced a saved job.
  neverCrawled: number
  // The most recent crawl over all links, or null when none ever ran.
  lastCrawledAt: string | null
}

export function portalViewNumbers(links: ViewLink[]): PortalViewNumbers {
  let withInstructions = 0
  let neverCrawled = 0
  let latest: string | null = null
  let latestTime = -Infinity
  for (const l of links) {
    if ((l.crawlInstructions ?? '').trim() !== '') withInstructions++
    if (!l.lastCrawledAt) {
      neverCrawled++
      continue
    }
    const time = new Date(l.lastCrawledAt).getTime()
    if (!Number.isNaN(time) && time > latestTime) {
      latestTime = time
      latest = l.lastCrawledAt
    }
  }
  return { links: links.length, withInstructions, neverCrawled, lastCrawledAt: latest }
}
