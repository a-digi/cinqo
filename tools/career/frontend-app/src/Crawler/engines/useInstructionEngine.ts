import { useCrawlEngine } from '../useCrawlEngine'
import { engineFor } from './registry'
import type { InstructionEngine } from './types'

// The instruction engine for the Browser's selected engine, for everything on a Platforms page (plan 192).
export function useInstructionEngine(): InstructionEngine {
  return engineFor(useCrawlEngine())
}
