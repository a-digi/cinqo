// Pure numbers for the Portals page's cards, worked out from the portals already loaded
// (no extra request). No React, so it can be script-checked.
import { hasListingFor, type EngineReading } from '../../Crawler/crawlEngine'

export interface LinkLike {
  // The backend leaves an empty field out of the JSON, so "none" arrives as undefined as well as null.
  crawlInstructions?: string | null
  // Plan 190: what the job list needs depends on the engine in use (a rule under Katana).
  linkRules?: unknown
  requiresLogin?: boolean
  loginSession?: string
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

export function summarizePortals(portals: PortalLike[], engine: EngineReading = null): PortalSummary {
  let links = 0
  let withInstructions = 0
  let neverCrawled = 0
  for (const p of portals) {
    for (const l of p.links) {
      links++
      if (
        hasListingFor(
          { crawlInstructions: l.crawlInstructions, linkRules: (l.linkRules ?? null) as never, requiresLogin: l.requiresLogin, loginSession: l.loginSession },
          engine,
        )
      )
        withInstructions++
      if (!l.lastCrawledAt) neverCrawled++
    }
  }
  return { portals: portals.length, links, withInstructions, neverCrawled }
}

export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}
