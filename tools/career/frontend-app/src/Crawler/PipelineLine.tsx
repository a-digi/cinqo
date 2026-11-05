import { useEffect, useRef, useState } from 'react'
import type { PipelineLink, PortalLink } from '../api'
import { PIPELINE_STEPS, isKnownStage, pipelineText, refreshDelayMs, runningFor, waitingText, type PipelineTone } from './pipelineStage'
import { loadPipeline } from './pipelineStore'

const TONE_CLASSES: Record<PipelineTone, { dot: string; text: string }> = {
  active: { dot: 'bg-amber-500 animate-pulse', text: 'text-amber-800' },
  waiting: { dot: 'bg-blue-500', text: 'text-blue-800' },
  attention: { dot: 'bg-red-500', text: 'text-red-800' },
  done: { dot: 'bg-green-500', text: 'text-green-800' },
  neutral: { dot: 'bg-gray-400', text: 'text-gray-600' },
}

// What makes the line ask again right now: anything on the link that a stage depends on. (The line also asks on a timer.)
function changeKey(link: PortalLink): string {
  return [
    link.crawlInstructions ? 1 : 0,
    link.jobDetailCrawlInstructions ? 1 : 0,
    link.listingCrawlRequested ? 1 : 0,
    link.jobDetailInstructionsAiRequested ? 1 : 0,
    link.jobDetailCrawlRequested ? 1 : 0,
    link.hasActiveCrawlRun ? 1 : 0,
    link.instructionsAiConversationId ?? '',
    link.jobDetailInstructionsAiConversationId ?? '',
    link.jobsQuestion ? 1 : 0,
    link.lastCrawledAt ?? '',
  ].join('|')
}

// One line in a link's panel (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 1): where the link is in the chain from the
// listing instructions to a complete set of jobs, and what it waits for. Four small marks show the four steps (listing instructions, listing
// crawl, job-detail instructions, job-detail crawl). Read-only: it only shows what the backend derived.
export function PipelineLine({ link }: { link: PortalLink }) {
  const [state, setState] = useState<PipelineLink | null>(null)
  const [failed, setFailed] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const key = changeKey(link)
  const loadedBefore = useRef(false)

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const load = (force: boolean) => {
      loadPipeline(force)
        .then((links) => {
          if (cancelled) return
          const mine = links.find((l) => l.portalLinkId === link.id) ?? null
          setState(mine)
          setFailed(false)
          setNow(Date.now())
          const tone = mine && isKnownStage(mine.stage) ? pipelineText(mine).tone : null
          timer = setTimeout(() => {
            load(false)
          }, refreshDelayMs(tone))
        })
        .catch(() => {
          if (cancelled) return
          setFailed(true)
          timer = setTimeout(() => {
            load(false)
          }, refreshDelayMs(null))
        })
    }
    // The first load shares the answer other panels just asked for; a later one (this link changed) asks again.
    load(loadedBefore.current)
    loadedBefore.current = true
    return () => {
      cancelled = true
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [link.id, key])

  if (failed && state === null) return null
  if (state === null || !isKnownStage(state.stage)) return null
  const text = pipelineText(state, { requiresLogin: link.requiresLogin })
  const tone = TONE_CLASSES[text.tone]
  const since = state.waitingFor === 'ai' || state.waitingFor === 'crawl' ? runningFor(state.since, now) : ''
  return (
    <div
      role="status"
      aria-label={`Pipeline: ${text.label}`}
      title={`${text.hint}\n${waitingText(state.waitingFor)}${since !== '' ? ` ${since}` : ''}`}
      className={`mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl bg-gray-50 px-3 py-1.5 text-xs ${tone.text}`}
    >
      <span className="flex items-center gap-1.5 font-medium">
        <span aria-hidden="true" className={`inline-block h-2 w-2 rounded-full ${tone.dot}`} />
        {text.label}
      </span>
      <span aria-hidden="true" className="flex items-center gap-1">
        {Array.from({ length: PIPELINE_STEPS }, (_, i) => (
          <span key={i} className={`h-1.5 w-5 rounded-full ${i < text.position ? 'bg-current opacity-70' : 'bg-gray-200'}`} />
        ))}
      </span>
      <span className="text-gray-500">
        {text.hint}
        {since !== '' ? ` (${since})` : ''}
      </span>
    </div>
  )
}
