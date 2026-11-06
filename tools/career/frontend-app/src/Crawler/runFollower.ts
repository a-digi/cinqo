import type { InstructionAttemptOutcome, PortalLink } from '../api'
import type { EngineReading } from './engines/types'
import { followActions, type RunKind } from './runFollowerRules'

// The run follower (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 4): lives in the tool's bundle, not in a page, so
// the follow-up of a run does not need the Platforms page. While the app is open in a tab it looks at the links every few seconds and
//  - finishes the bookkeeping of an AI run whose turn nobody follows (page closed or reloaded): records the attempt, clears the conversation
//    id, records the error, exactly as a page that watched the run would;
//  - consumes the two crawl flags (listing crawl, job-detail crawl) that the backend's events raise, once no run of the link goes on.
// A run a page of this tab carries itself (its own closure finishes it) is registered with setPageBusy and left alone: one owner per run.
// Starting a new AI run is not done here; it needs the page's choice of AI platform and model.
export interface FollowerDeps {
  fetchLinks: () => Promise<PortalLink[]>
  // The Browser's selected engine and whether a link has what its job list needs under it (plan 192). A read that fails means "not known".
  readEngine: () => Promise<EngineReading>
  hasListing: (link: PortalLink, engine: EngineReading) => boolean
  updateLink: (linkId: string, patch: Record<string, string | boolean>) => Promise<unknown>
  startCrawl: (linkId: string, kind: RunKind) => Promise<unknown>
  awaitTurn: (conversationId: string) => Promise<unknown>
  recordOutcome: (linkId: string, kind: RunKind, errorText?: string) => Promise<InstructionAttemptOutcome | undefined>
  errorText: (err: unknown, kind: RunKind) => string
  // Called after the follower changed something, so a page that is open can reload.
  onChange: () => void
}

export interface RunFollower {
  start: () => void
  stop: () => void
  poke: () => void
  tick: () => Promise<void>
  setPageBusy: (linkId: string, kind: RunKind, busy: boolean) => void
  subscribe: (listener: () => void) => () => void
}

const TICK_MS = 10000
const FIRST_TICK_MS = 3000
const POKE_MS = 400

export function createRunFollower(deps: FollowerDeps): RunFollower {
  const pageBusy = new Map<string, Set<RunKind>>()
  const followed = new Set<string>()
  let lastSeen = new Map<string, string>() // the facts of every link at the last look, to tell a page when the backend changed one
  const listeners = new Set<() => void>()
  let timer: ReturnType<typeof setInterval> | undefined
  let first: ReturnType<typeof setTimeout> | undefined
  let poking: ReturnType<typeof setTimeout> | undefined
  let ticking = false

  function changed() {
    deps.onChange()
    for (const listener of listeners) listener()
  }

  async function finishRun(link: PortalLink, kind: RunKind, err: unknown): Promise<void> {
    const listing = kind === 'listing'
    const clear: Record<string, string | boolean> = listing
      ? { instructionsAiError: '', instructionsAiConversationId: '' }
      : { jobDetailInstructionsAiError: '', jobDetailInstructionsAiConversationId: '', jobDetailInstructionsAiRequested: false }
    const ignore = () => undefined
    if (!err) {
      if (listing) {
        // A turn that ended on the AI's question is waiting for the user: nothing is finished.
        const fresh = await deps.fetchLinks().catch(() => [] as PortalLink[])
        if (fresh.find((l) => l.id === link.id)?.jobsQuestion) return
      }
      const recorded = deps.recordOutcome(link.id, kind)
      await deps.updateLink(link.id, clear).catch(ignore)
      await recorded
      return
    }
    const text = deps.errorText(err, kind)
    const outcome = await deps.recordOutcome(link.id, kind, text)
    // The backend's verdict decides: a turn can end in an error after the instructions were saved.
    if (outcome === 'succeeded') {
      await deps.updateLink(link.id, clear).catch(ignore)
      return
    }
    // 'none': no attempt was running any more, so someone else (a page that carried the run) already finished it and recorded the verdict;
    // writing this turn's error now would overwrite that verdict (plan 192, step 10: a run that saved and then ran out of rounds showed its
    // turn error although its attempt succeeded).
    if (outcome === 'none') return
    const failure: Record<string, string> = listing
      ? { instructionsAiError: text, instructionsAiConversationId: '' }
      : { jobDetailInstructionsAiError: text, jobDetailInstructionsAiConversationId: '' }
    await deps.updateLink(link.id, failure).catch(ignore)
  }

  async function follow(link: PortalLink, kind: RunKind, conversationId: string): Promise<void> {
    followed.add(conversationId)
    let failure: unknown
    try {
      await deps.awaitTurn(conversationId)
    } catch (err) {
      failure = err ?? new Error('The AI run failed.')
    }
    try {
      await finishRun(link, kind, failure)
    } catch (err) {
      console.error('failed to finish the instruction run of portal link', link.id, err)
    }
    changed()
  }

  async function tick(): Promise<void> {
    if (ticking) return
    ticking = true
    try {
      let links: PortalLink[]
      try {
        links = await deps.fetchLinks()
      } catch {
        return
      }
      // What the backend changed by itself (a crawl that ended and raised the job-detail request, a run that finished) reaches an open page only
      // when it reloads; the page's own effects (the automatic start of the AI for the job details) read what the link says. So when the facts
      // they act on differ from the last look, the page is told to reload (plan 192: the chain after a crawl must not wait for a manual reload).
      let somethingChanged = false
      const seenNow = new Map<string, string>()
      for (const l of links) {
        const signature = [
          l.listingCrawlRequested,
          l.jobDetailInstructionsAiRequested,
          l.jobDetailCrawlRequested,
          l.hasActiveCrawlRun,
          l.lastCrawledAt ?? '',
          l.jobDetailCrawlInstructions ?? '',
          l.linkRules ? 'rule' : '',
        ].join('|')
        seenNow.set(l.id, signature)
        if (lastSeen.has(l.id) && lastSeen.get(l.id) !== signature) somethingChanged = true
      }
      lastSeen = seenNow
      if (somethingChanged) changed()

      // A conversation that no link names any more is forgotten.
      const named = new Set(links.flatMap((l) => [l.instructionsAiConversationId ?? '', l.jobDetailInstructionsAiConversationId ?? '']))
      for (const id of [...followed]) if (!named.has(id)) followed.delete(id)

      const engine = await deps.readEngine().catch((): EngineReading => null)
      for (const link of links) {
        const actions = followActions(link, {
          pageBusy: (id) => (pageBusy.get(id)?.size ?? 0) > 0,
          followed: (id) => followed.has(id),
          hasListing: (l) => deps.hasListing(l as PortalLink, engine),
        })
        for (const action of actions) {
          if (action.kind === 'resume_run') {
            void follow(link, action.runKind, action.conversationId)
            continue
          }
          const kind: RunKind = action.kind === 'listing_crawl' ? 'listing' : 'job_detail'
          const flag: Record<string, boolean> = kind === 'listing' ? { listingCrawlRequested: false } : { jobDetailCrawlRequested: false }
          // The flag is cleared first (as a page did): a failed start never loops.
          await deps.updateLink(link.id, flag).catch(() => undefined)
          await deps.startCrawl(link.id, kind).catch(() => undefined)
          changed()
        }
      }
    } finally {
      ticking = false
    }
  }

  return {
    start() {
      if (timer !== undefined) return
      first = setTimeout(() => void tick(), FIRST_TICK_MS)
      timer = setInterval(() => void tick(), TICK_MS)
    },
    stop() {
      if (timer !== undefined) clearInterval(timer)
      if (first !== undefined) clearTimeout(first)
      if (poking !== undefined) clearTimeout(poking)
      timer = first = poking = undefined
    },
    poke() {
      if (poking !== undefined) return
      poking = setTimeout(() => {
        poking = undefined
        void tick()
      }, POKE_MS)
    },
    tick,
    setPageBusy(linkId, kind, busy) {
      const kinds = pageBusy.get(linkId) ?? new Set<RunKind>()
      if (busy) kinds.add(kind)
      else kinds.delete(kind)
      if (kinds.size === 0) pageBusy.delete(linkId)
      else pageBusy.set(linkId, kinds)
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
  }
}
