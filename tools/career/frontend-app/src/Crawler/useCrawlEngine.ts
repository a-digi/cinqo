import { createContext, useContext } from 'react'
import type { EngineReading } from './crawlEngine'

// The selected Browser engine for everything on a Platforms page (plan 190), provided by CrawlEngineProvider. null until the first answer:
// every rule then judges nothing by the engine.
export const CrawlEngineContext = createContext<EngineReading>(null)

export function useCrawlEngine(): EngineReading {
  return useContext(CrawlEngineContext)
}
