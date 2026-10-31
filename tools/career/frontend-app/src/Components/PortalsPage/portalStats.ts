// Pure numbers for the Portals page's cards, worked out from the portals already loaded
// (no extra request). No React, so it can be script-checked.
export interface LinkLike {
  // The backend leaves an empty field out of the JSON, so "none" arrives as undefined as well as null.
  crawlInstructions?: string | null
  lastCrawledAt?: string | null
}

export interface PortalLike {
  links: LinkLike[]
}

export interface PortalSummary {
  portals: number
  links: number
  // Links that have crawl instructions, i.e. that can actually be crawled.
  withInstructions: number
  // Links that have never produced a saved job.
  neverCrawled: number
}

export function summarizePortals(portals: PortalLike[]): PortalSummary {
  let links = 0
  let withInstructions = 0
  let neverCrawled = 0
  for (const p of portals) {
    for (const l of p.links) {
      links++
      if ((l.crawlInstructions ?? '').trim() !== '') withInstructions++
      if (!l.lastCrawledAt) neverCrawled++
    }
  }
  return { portals: portals.length, links, withInstructions, neverCrawled }
}

export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}
