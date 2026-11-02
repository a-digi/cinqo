import { fetchPipeline, type PipelineLink } from '../api'

// One shared load of the pipeline for every link panel on the page (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md): the
// route answers for all links at once, so a page with twenty links asks once, not twenty times. A load younger than FRESH_MS is reused.
const FRESH_MS = 1500

let cached: { at: number; promise: Promise<PipelineLink[]> } | null = null

export function loadPipeline(force = false): Promise<PipelineLink[]> {
  if (!force && cached && Date.now() - cached.at < FRESH_MS) return cached.promise
  const promise = fetchPipeline()
  cached = { at: Date.now(), promise }
  // A failed load is not kept: the next ask tries again.
  promise.catch(() => {
    if (cached?.promise === promise) cached = null
  })
  return promise
}
