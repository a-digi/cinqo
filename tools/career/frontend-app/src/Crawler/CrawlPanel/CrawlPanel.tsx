import { useEffect, useRef, useState } from 'react'
import {
  updatePortalLink,
  fetchPortals,
  type InstructionAttemptOutcome,
  startInstructionAttempt,
  finishInstructionAttempt,
  type InstructionKind,
  type PortalLink,
} from '../../api'
import { Dropdown } from '../../Components/Dropdown/Dropdown'
import { Switch } from '../../Shared/Switch/Switch'
import { AUTO_DISCOVERY_INTERVAL_OPTIONS } from '../autoDiscovery'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, deleteConversation, sendMessage } from '../../Cinqo/Conversation/conversation'
import { restrictionHolds } from '../../PersonaAi/personaTools'
import { buildCrawlMessage, crawlConversationTitle } from '../crawl'
import {
  buildGenerateInstructionsMessage,
  buildJobsConfirmedMessage,
  GENERATE_JOB_DETAIL_TOOLS,
  GENERATE_LISTING_TOOLS,
  GENERATE_RESTRICTION_NOT_APPLIED_MESSAGE,
  GENERATE_TOOL_ITERATION_LIMIT,
  generateInstructionsConversationTitle,
  buildGenerateJobDetailInstructionsMessage,
  generateJobDetailInstructionsConversationTitle,
  GENERATE_LISTING_WITH_DETAIL_TOOLS,
  extractSampleJobUrl,
  buildJobDetailFollowUpMessage,
  hasJobDetailInstructions,
  shouldContinueWithJobDetail,
} from '../generateInstructions'
import { startCrawlNow, stopCrawlNow, fetchActiveCrawlRun, type CrawlRun } from '../crawlNow'
import { LinkCrawlPanel } from '../LinkCrawl/LinkCrawlPanel'
import { JobDetailPickPanel } from '../JobDetailPick/JobDetailPickPanel'
import { phaseLabel, latestCrawlLogMessage } from '../crawlPhase'
import {
  AI_LOGIN_WAIT_TEXT,
  LoginWaitCancelled,
  closeLoginWindow,
  confirmLogin,
  generateLoginNote,
  isWaitingForLogin,
  manualOnlyNote,
  openAiLoginWindow,
} from '../loginWindow'
import { instructionAlertAttempts, isLikelyUncrawlable, listingIsWrittenByAi, savedDespiteError } from '../instructionStats'
import { canCrawlListing } from '../autoDiscoveryVisibility'
import { hasListingFor, jobDetailIsWrittenByAi, listingNeeds, mayAutoWriteJobDetails } from '../crawlEngine'
import { useCrawlEngine } from '../useCrawlEngine'
import { PipelineLine } from '../PipelineLine'
import { runFollower } from '../runFollowerStart'
import { LoginGenerateAbandoned, startAfterLogin } from '../loginGenerate'
import { diagnoseEmptyCrawl, emptyCrawlFacts, emptyCrawlFor } from '../emptyCrawl'
import { shouldTryInherit, type InheritView } from '../inheritOutcome'
import { FREE_ROUTER_HINT, isFreeRouter } from '../modelHint'
import { findJobsQuestion, pendingJobsQuestion, JOBS_QUESTION_NO_HINT, JOBS_QUESTION_TITLE, JOBS_QUESTION_YES_HINT } from '../jobsQuestion'
import { InstructionAttemptStats } from './InstructionAttemptStats'
import { StartFreshButton } from './StartFreshButton'
import { AccordionItem } from '../../Shared/Accordion/AccordionItem'
import {
  PlayIcon,
  StopIcon,
  LogIcon,
  AlertIcon,
  RobotIcon,
  CrawlInstructionsIcon,
  JobDetailInstructionsIcon,
} from '../../Shared/Icons/icons'
import { Modal } from '../../Shared/Modal/Modal'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'

// A crawl run's own log is sparse (four or five entries total), so a
// tighter poll than this (coarser than the AI conversation feature's
// own 2s turn poll) would buy nothing.
const CRAWL_POLL_INTERVAL_MS = 3000

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// shouldShowError decides whether a durable AI-generation error banner
// should still show, given when it was last dismissed (step 71/72):
// dismissedAt at least as recent as errorAt means "stay hidden" — a
// NEWER errorAt (a fresh failure) means it should show again. String
// comparison is safe here since every one of these timestamps is
// already stored/serialized in a consistently-ordered format, the same
// assumption every existing `new Date(link.xAt)` call site in this file
// already makes. See
// plan/ai/tools/career/step-72-wire-infobox-into-crawlpanel.md.
function shouldShowError(errorAt: string | null | undefined, dismissedAt: string | null | undefined): boolean {
  if (!errorAt) return false
  if (!dismissedAt) return true
  return dismissedAt < errorAt
}

// CrawlPanel — one instance per portal link (extracted out of
// PortalsPage.tsx, step 42), owning everything about crawling THIS one
// link: two independent manual-trigger mechanisms ("Crawl now" —
// deterministic, no AI, runs detached server-side, steps 27/37/38 —
// and "Crawl with AI" — an AI-driven conversation, steps 24-26/27),
// this link's own stored crawl_instructions view/edit, and
// "Generate with AI" (step 33) to have the AI write them. See
// plan/ai/tools/career/step-21-portals-frontend.md and
// plan/ai/tools/career/step-42-crawler-folder-reorganization.md.
export function CrawlPanel({
  link,
  selectedPlatform,
  selectedModel,
  hasPlatforms,
  inheriting,
  inheritNote,
  onTryInherit,
  onReload,
  onError,
}: {
  link: PortalLink
  selectedPlatform: Platform | null
  selectedModel: string | null
  hasPlatforms: boolean
  // Step 103: a trial of the portal's other instructions is running for this link; what the last
  // one said; and how to run one (resolves true when the listing instructions were inherited).
  inheriting: boolean
  inheritNote: InheritView | null
  onTryInherit: () => Promise<boolean>
  onReload: () => void
  onError: (message: string) => void
}) {
  // Plan 190: the Browser's selected engine decides which job-list instructions count (a rule under Katana, AI instructions under Rod).
  const engine = useCrawlEngine()
  // "Crawl with AI" own busy/result state.
  const [isCrawling, setIsCrawling] = useState(false)
  const [crawlResult, setCrawlResult] = useState<{ ok: boolean; text: string; conversationId?: string } | null>(null)

  // "Crawl now" own live state (steps 37/38) — the authoritative
  // record of what Career's own detached backend goroutine has
  // reported for this link's most recent run, whether still running
  // or already terminal. Independent of isCrawling/crawlResult above
  // (those belong to "Crawl with AI" only).
  const [run, setRun] = useState<CrawlRun | null>(null)
  // Cancellation token (same pattern ConversationContext.tsx's own
  // watchTokensRef uses) — lets "Stop watching" supersede an
  // in-flight poll loop cleanly without an AbortController.
  const watchTokenRef = useRef(0)
  const [logExpanded, setLogExpanded] = useState(false)
  // isStoppingCrawl (step 63) — true only while the real "Stop crawl"
  // request itself is in flight, so a double-click can't fire two
  // cancel requests. Independent of isBusy()/isCrawling — "Stop
  // watching" and "Crawl with AI" both remain clickable while this is
  // true.
  const [isStoppingCrawl, setIsStoppingCrawl] = useState(false)

  // isBusy is the one place "is this link already crawling" is
  // decided — true while either mechanism is active, so starting one
  // disables the other's own button.
  function isBusy(): boolean {
    return isCrawling || run?.status === 'running'
  }

  // autoDiscoveryPending (step 84 follow-up) — true only while THIS
  // toggle's own write is in flight, so a double-click can't fire two
  // updates; independent of isBusy() above, since opting a link in or
  // out of auto-discovery has nothing to do with whether it's
  // currently crawling.
  const [autoDiscoveryPending, setAutoDiscoveryPending] = useState(false)
  function handleToggleAutoDiscovery(nextEnabled: boolean) {
    setAutoDiscoveryPending(true)
    updatePortalLink(link.id, { autoDiscoveryEnabled: nextEnabled })
      .then(onReload)
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : 'Failed to update auto-discovery for this link.')
      })
      .finally(() => {
        setAutoDiscoveryPending(false)
      })
  }

  // AUTO_DISCOVERY_GLOBAL_VALUE (step 85) is the Dropdown's own "use
  // the global interval" option — a plain empty string, not a real
  // interval choice, matching the same "" = unselected convention
  // this tool's other Dropdown call sites already use (e.g. JobsPage's
  // own "Any company"/"Any location"). Selecting it clears this link's
  // own override (clearAutoDiscoveryInterval), never sends it as a
  // literal interval value.
  const AUTO_DISCOVERY_GLOBAL_VALUE = ''
  function handleChangeAutoDiscoveryInterval(value: string) {
    setAutoDiscoveryPending(true)
    const write =
      value === AUTO_DISCOVERY_GLOBAL_VALUE
        ? updatePortalLink(link.id, { clearAutoDiscoveryInterval: true })
        : updatePortalLink(link.id, { autoDiscoveryIntervalMinutes: Number(value) })
    write
      .then(onReload)
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : "Failed to update this link's own auto-discovery interval.")
      })
      .finally(() => {
        setAutoDiscoveryPending(false)
      })
  }

  // "Generate with AI" (step 33) own busy/error state — independent of
  // isCrawling above: generating/editing this link's own
  // crawl_instructions is a different operation from crawling it, and
  // could in principle overlap. aiLocalError is a same-session,
  // immediate failure display, set the moment the AI attempt itself
  // fails, never waiting on (or depending on) the durable
  // updatePortalLink(instructionsAiError) write that follows it also
  // succeeding — without this, a failure of that *recording* call
  // itself (a separate network request, which can fail for its own
  // reasons) would leave nothing visible at all. link.instructionsAiError
  // (the durable, reload-surviving field from the portals payload) is
  // still the source of truth once onReload() refreshes; this local
  // copy is only a same-session guarantee that a failure is never
  // silently invisible. See
  // plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
  const [aiPending, setAiPendingState] = useState(false)
  // Registered with the run follower: a run this page carries itself is finished by this page's own code, even after the page is left, so
  // the follower stays out of it (plan 163, step 4).
  function setAiPending(busy: boolean) {
    setAiPendingState(busy)
    runFollower.setPageBusy(link.id, 'listing', busy)
  }
  const [aiLocalError, setAiLocalError] = useState<string | undefined>(undefined)
  // plan 183: Generate for a link that needs a login. aiLoginWait is the id of the conversation whose login window waits for the person
  // (null otherwise); the ref holds the promise's resolvers the card's buttons settle; aiLoginWindowRef is the window to close when the run
  // ends. The conversation id is stored on the link only AFTER the confirmation, so the run follower never sees a run that has not started.
  const [aiLoginWait, setAiLoginWait] = useState<string | null>(null)
  const aiLoginWaitRef = useRef<{ resolve: () => void; reject: (err: Error) => void } | null>(null)
  const aiLoginWindowRef = useRef<string | null>(null)
  function closeAiLoginWindow() {
    const id = aiLoginWindowRef.current
    aiLoginWindowRef.current = null
    if (id !== null) void closeLoginWindow(id)
  }
  function waitForPersonToLogin(conversationId: string): Promise<void> {
    setLoginConfirmError('')
    setAiLoginWait(conversationId)
    return new Promise<void>((resolve, reject) => {
      aiLoginWaitRef.current = {
        resolve: () => {
          aiLoginWaitRef.current = null
          setAiLoginWait(null)
          resolve()
        },
        reject: (err: Error) => {
          aiLoginWaitRef.current = null
          setAiLoginWait(null)
          reject(err)
        },
      }
    })
  }
  // The conversation "Check Progress - AI" reopens (step 60) — set the
  // moment one is created (fresh run) or discovered still running on
  // mount (resumed run), cleared once it finishes either way.
  const [aiConversationId, setAiConversationId] = useState<string | null>(null)

  // activeInstructionsModal replaces this file's own former pair of
  // independent expand-booleans (instructionsExpanded/
  // jobDetailInstructionsExpanded) — the view/edit UI moved from an
  // inline expanding textarea to a shared Modal (below), and only one
  // of the two documents can ever be open at a time now that both share
  // the one modal, so a single tri-state value is simpler and cannot
  // represent "both open" (a state the old two-boolean shape could,
  // incorrectly, have allowed). See
  // plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
  const [activeInstructionsModal, setActiveInstructionsModal] = useState<'crawl' | 'rules' | 'jobDetail' | null>(null)
  const [instructionsDraft, setInstructionsDraft] = useState('')

  // crawlInstructionsOpen — the "Crawl instructions" section's own
  // accordion state. Initialized ONCE from whether this link already
  // has listing crawl instructions (closed — already configured,
  // nothing needs attention) or not (open — this is exactly what needs
  // the user's attention). A lazy initializer, not a value kept in sync
  // with link.crawlInstructions on every re-render: once open or
  // closed, only the user's own click (below) changes it again — an
  // AI-driven regeneration finishing later must not yank a section the
  // user deliberately opened back shut, or vice versa. See
  // plan/ai/tools/career/step-XX-crawl-instructions-accordion.md.
  const [crawlInstructionsOpen, setCrawlInstructionsOpen] = useState(() => !hasListingFor(link, engine))

  // jobDetailInstructionsDraft — the SEPARATE, second instruction
  // document's own draft (a single job's own detail page, not the
  // listing page above) — kept as its own independent state rather
  // than reusing instructionsDraft: the two documents are edited
  // independently and opening one must never clobber the other's own
  // unsaved draft.
  const [jobDetailInstructionsDraft, setJobDetailInstructionsDraft] = useState('')

  // "Generate with AI" for the job-detail instructions — the SEPARATE
  // counterpart to aiPending/aiLocalError/aiConversationId above, same
  // reasoning as jobDetailInstructionsExpanded/jobDetailInstructionsDraft:
  // generating one document with AI must never disable or clobber the
  // other's own independent in-flight state.
  const [jobDetailAiPending, setJobDetailAiPendingState] = useState(false)
  function setJobDetailAiPending(busy: boolean) {
    setJobDetailAiPendingState(busy)
    runFollower.setPageBusy(link.id, 'job_detail', busy)
  }
  const [jobDetailAiLocalError, setJobDetailAiLocalError] = useState<string | undefined>(undefined)
  const [jobDetailAiConversationId, setJobDetailAiConversationId] = useState<string | null>(null)

  // Resume watching a "Crawl now" run still in progress — the concrete
  // mechanism behind "the user can leave the page, it does not have to
  // be in the tab": reopening/reloading PortalsPage re-fetches
  // portals, which brings this link's own hasActiveCrawlRun back, and
  // this effect immediately starts polling if it's still running,
  // without requiring a click. resumedRef (not watchTokenRef) guards
  // this: once true, never reset, so an unrelated reload elsewhere on
  // the page (e.g. editing a different link) never reopens a watch
  // this instance's own "Stop watching" just explicitly closed —
  // hasActiveCrawlRun stays true on the server regardless of what this
  // tab's UI decided to stop showing, so watching
  // link.hasActiveCrawlRun directly (without this guard) would reopen
  // it immediately on the very next unrelated load(). This is a real
  // bug caught while first writing this effect (steps 37/38), not a
  // hypothetical.
  const resumedRef = useRef(false)
  useEffect(() => {
    if (link.hasActiveCrawlRun && !resumedRef.current) {
      resumedRef.current = true
      void watchCrawlRun()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.hasActiveCrawlRun])

  // Plan 163, step 4: a run of the instructions whose page is gone (closed or reloaded) is finished by the run follower (Crawler/runFollower.ts),
  // and so are the listing-crawl and job-detail-crawl flags: this panel only shows what the link says. What a page of this tab carries
  // itself is registered with the follower (setAiBusy / setJobDetailAiBusy below), so each run has one owner. The follower is asked to look
  // now whenever one of those facts changes.
  useEffect(() => {
    runFollower.poke()
  }, [
    link.listingCrawlRequested,
    link.jobDetailCrawlRequested,
    link.jobDetailCrawlInstructions,
    link.instructionsAiConversationId,
    link.jobDetailInstructionsAiConversationId,
    link.hasActiveCrawlRun,
  ])

  // jobDetailAiRequestedRef consumes link.jobDetailInstructionsAiRequested
  // (step 67) the same way — set by a SEPARATE listener once a
  // triggered crawl finishes and this link still has no job-detail
  // instructions. Deliberately does NOT clear the flag itself: it's
  // only cleared as a side effect of
  // finishGenerateJobDetailInstructions's own success path below, so a
  // failure leaves it set and simply reopening the page retries
  // automatically — there is no manual fallback button anymore (step
  // 68). The !link.jobDetailCrawlInstructions guard protects against a
  // stale flag that survived past a document a human already wrote by
  // hand via the view/edit modal.
  const jobDetailAiRequestedRef = useRef(false)
  const jobDetailAiClearedRef = useRef(false)
  useEffect(() => {
    // Plan 190: not before the engine is known, and not under Katana, where the job page is read by picking its data. A request left over
    // from before (or raised by the crawl's event, which knows no engine) is cleared under Katana, so that switching to Rod later does not
    // start an AI run nobody asked for.
    if (engine === 'katana' && !link.requiresLogin) {
      if (link.jobDetailInstructionsAiRequested && !jobDetailAiClearedRef.current) {
        jobDetailAiClearedRef.current = true
        void updatePortalLink(link.id, { jobDetailInstructionsAiRequested: false }).then(onReload)
      }
      return
    }
    if (!mayAutoWriteJobDetails(engine, link.requiresLogin)) return
    if (link.jobDetailInstructionsAiRequested && !link.jobDetailCrawlInstructions && !jobDetailAiRequestedRef.current) {
      jobDetailAiRequestedRef.current = true
      handleGenerateJobDetailInstructionsWithAI()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.jobDetailInstructionsAiRequested, link.jobDetailCrawlInstructions, engine])

  // watchCrawlRun polls GET .../crawl-now/active until the run reaches
  // a terminal status, writing every observed row into `run` as it
  // goes — this is what "the frontend shows the status and the logs"
  // means concretely: `run` is always exactly what the backend's own
  // crawl_runs row says, never optimistic local state.
  async function watchCrawlRun() {
    const myToken = watchTokenRef.current + 1
    watchTokenRef.current = myToken

    for (;;) {
      let latestRun: CrawlRun | null
      try {
        latestRun = await fetchActiveCrawlRun(link.id)
      } catch {
        break
      }
      if (watchTokenRef.current !== myToken) return // superseded
      if (!latestRun) break
      setRun(latestRun)
      if (latestRun.status !== 'running') break
      await sleep(CRAWL_POLL_INTERVAL_MS)
      if (watchTokenRef.current !== myToken) return
    }
  }

  // "Crawl now" (steps 27/37/38) — starts the crawl on Career's own
  // backend and begins watching it; the actual navigate/extract/ingest
  // sequence runs entirely server-side from this point on; closing
  // this tab does not stop it (see step-37's own design). A start
  // failure (e.g. 409 — one already running, or 400 — no valid crawl
  // instructions) is shown via crawlResult, the same place "Crawl with
  // AI" shows its own outcomes; the run's own later live/terminal
  // state always lives in `run` instead.
  function handleCrawlNow() {
    if (isBusy()) return
    setCrawlResult(null)
    startCrawlNow(link.id)
      .then((started) => {
        setRun({
          crawlRunId: started.crawlRunId,
          kind: 'listing',
          triggeredBy: 'manual',
          status: 'running',
          startedAt: started.startedAt,
          finishedAt: null,
          log: [],
          resultSummary: null,
          errorMessage: null,
          phase: null,
        })
        return watchCrawlRun()
      })
      .catch((err: unknown) => {
        setCrawlResult({ ok: false, text: err instanceof Error ? err.message : 'Failed to start crawl.' })
      })
  }

  // "Stop watching" only ever bumps the token so this instance stops
  // polling and clears its own local view of the run — it does NOT
  // cancel anything server-side. (Until step 63, Career's own
  // goroutine had no cancel path at all; handleStopCrawlNow, below, is
  // the real, server-effecting stop this button was never able to be.)
  // Reopening this page later re-discovers the run via
  // hasActiveCrawlRun/the resume effect above if it's still going, or
  // shows its finished result if it already completed.
  function handleCancelCrawlNow() {
    watchTokenRef.current += 1
    setRun(null)
    setCrawlResult({
      ok: false,
      text: 'Stopped watching in this tab — the crawl may still be running in the background; reopen this page to check its latest status.',
    })
  }

  // "Stop crawl" (step 63) — a real, server-effecting cancellation:
  // interrupts both Career's own detached goroutine and, in turn, the
  // browser tool's in-flight chromedp work for it. Fires immediately,
  // no confirmation — matches "Stop watching"'s own immediate
  // behavior, and the action is safe (stops work, destroys no data).
  // See plan/ai/tools/career/step-63-stop-crawling-now.md.
  // plan 182: "I am logged in" — the Browser tool checks the page in the run's window and, when it is no longer a login page, lets the run
  // crawl. A refusal (still a login form, a bot check) is shown under the button; nothing typed in the window is ever seen here.
  const [isConfirmingLogin, setIsConfirmingLogin] = useState(false)
  const [loginConfirmError, setLoginConfirmError] = useState('')
  function handleConfirmLogin() {
    if (!run) return
    setIsConfirmingLogin(true)
    setLoginConfirmError('')
    confirmLogin(run.crawlRunId)
      .catch((err: unknown) => {
        setLoginConfirmError(err instanceof Error ? err.message : 'Could not confirm the login.')
      })
      .finally(() => {
        setIsConfirmingLogin(false)
      })
  }

  function handleConfirmAiLogin() {
    if (aiLoginWait === null) return
    setIsConfirmingLogin(true)
    setLoginConfirmError('')
    confirmLogin(aiLoginWait)
      .then(() => {
        aiLoginWaitRef.current?.resolve()
      })
      .catch((err: unknown) => {
        setLoginConfirmError(err instanceof Error ? err.message : 'Could not confirm the login.')
      })
      .finally(() => {
        setIsConfirmingLogin(false)
      })
  }

  function handleCancelAiLogin() {
    aiLoginWaitRef.current?.reject(new LoginWaitCancelled())
  }

  function handleStopCrawlNow() {
    setIsStoppingCrawl(true)
    stopCrawlNow(link.id)
      .then((cancelled) => {
        if (cancelled) {
          // Merged into the EXISTING run, not replaced wholesale — the
          // cancel endpoint only ever returns {crawlRunId, status},
          // not a full CrawlRun; this preserves the log/phase the
          // panel was already showing at the moment of cancellation.
          setRun((prev) => (prev ? { ...prev, status: 'cancelled', finishedAt: new Date().toISOString() } : prev))
        } else {
          // 404 — nothing was active to cancel; the run already reached
          // a terminal state on its own in the gap between this click
          // and the request landing. Not a failure: re-fetch to show
          // whatever it actually became instead of leaving stale
          // 'running' state on screen.
          fetchActiveCrawlRun(link.id)
            .then(setRun)
            .catch(() => {
              // Best-effort refresh — nothing sensible to show here
              // beyond what the failed generic .catch below already
              // reports if the original stopCrawlNow call itself
              // fails; this inner one only re-fetches after a 404.
            })
        }
        // Supersede any in-flight watchCrawlRun poll loop — it would
        // otherwise race this update on its own next tick with a
        // possibly-stale intermediate read.
        watchTokenRef.current += 1
      })
      .catch((err: unknown) => {
        setCrawlResult({ ok: false, text: err instanceof Error ? err.message : 'Failed to stop crawl.' })
      })
      .finally(() => {
        setIsStoppingCrawl(false)
      })
  }

  // "Crawl with AI" (steps 24-26, kept per step 27) — creates a
  // conversation and lets the AI itself drive browser's tools, using
  // its own judgement rather than the fixed label vocabulary above.
  function handleCrawlWithAI() {
    if (isBusy() || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setIsCrawling(true)
    setCrawlResult(null)

    createConversation({ title: crawlConversationTitle(link), platformId, model })
      .then((conversation) => sendMessage(conversation.id, buildCrawlMessage(link)).then((result) => ({ conversation, result })))
      .then(({ conversation, result }) => {
        const suffix = result.durationMs != null ? ` (${(result.durationMs / 1000).toFixed(1)}s)` : ''
        setCrawlResult({ ok: true, text: `${result.content}${suffix}`, conversationId: conversation.id })
      })
      .catch((err: unknown) => {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'Crawl failed.'
        setCrawlResult({ ok: false, text })
      })
      .finally(() => {
        setIsCrawling(false)
      })
  }

  // recordAttemptFinish (step 86) tells the backend a Generate-with-AI
  // run for this kind just ended; the backend decides whether it
  // succeeded (an AI save tool saved instructions during the run) —
  // errorText is only the reason to show if it did not. Never rejects:
  // counting must never break the run's own finish handling, and the
  // returned promise lets callers wait for it before reloading so the
  // fresh counts are what the reload shows.
  function recordAttemptFinish(kind: InstructionKind, errorText?: string): Promise<void> {
    return recordAttemptOutcome(kind, errorText).then(() => undefined)
  }

  // recordAttemptOutcome is recordAttemptFinish that also returns the backend's
  // verdict (undefined when it could not be read). Step 95: a turn that ended
  // in an error may still have saved the instructions on its last round; the
  // verdict, not the turn, decides whether that is shown as a failure.
  function recordAttemptOutcome(kind: InstructionKind, errorText?: string): Promise<InstructionAttemptOutcome | undefined> {
    return finishInstructionAttempt(link.id, kind, errorText).catch((recordErr: unknown) => {
      console.error('failed to record instruction attempt finish for portal link', link.id, recordErr)
      return undefined
    })
  }

  // recordAttemptStart (step 86) — same never-rejects contract: a
  // failure to count an attempt must not stop the attempt itself. The
  // reload that follows (step 89) is what turns this link's server-side
  // "running" flag on in the link's accordion header, which sits outside
  // this component and has no access to aiPending — without it a stale
  // "probably can't be crawled" pill would linger during a retry.
  function recordAttemptStart(kind: InstructionKind, triggeredBy: 'manual' | 'auto'): Promise<void> {
    return startInstructionAttempt(link.id, kind, triggeredBy)
      .catch((recordErr: unknown) => {
        console.error('failed to record instruction attempt start for portal link', link.id, recordErr)
      })
      .then(() => {
        onReload()
      })
  }

  // finishGenerateInstructions is the shared "a generate-instructions
  // run just ended" handling (step 60) — used by both a fresh run
  // (handleGenerateInstructionsWithAI below) and a resumed one
  // (resumeGenerateInstructions), so a run that finishes while this
  // tab was away is handled identically to one that finishes while
  // it's open. err is the failure from awaitTurnCompletion, if any;
  // undefined means success.
  //
  // followUp (plan 163, step 3) is the conversation and the final reply of a run that started for a link without job-detail instructions: when its
  // listing instructions were really saved and the reply named a sample job, the run goes on in that same conversation with the job-detail
  // instructions (continueWithJobDetail).
  function finishGenerateInstructions(err?: unknown, followUp?: { conversationId: string; reply: string }) {
    // A run that may still go on stays "pending" until that is decided, so the listing crawl that waits for it (listingCrawlRequestedRef)
    // cannot slip in between the two.
    if (err || followUp === undefined) setAiPending(false)
    setAiConversationId(null)
    // The login window of a link that needs a login closes when the run ends: now on a failure; below on a success unless the run goes on (the
    // AI asked a question and waits for the answer, or the job-detail instructions follow in the same conversation).
    if (err) closeAiLoginWindow()

    if (!err) {
      // Step 91: a turn that ended on the agent's question is neither a
      // success nor a failure — it is waiting for the user's answer. The
      // attempt stays open and the conversation id is kept, because the
      // answer is sent into that same conversation. Any failure to read
      // the portals falls through to the ordinary success handling.
      void fetchPortals()
        .then((portals) => ({
          question: findJobsQuestion(portals, link.id),
          // The link as it is now, to see whether it already has job-detail instructions.
          fresh: portals.flatMap((portal) => portal.links).find((l) => l.id === link.id),
        }))
        .catch(() => ({ question: undefined, fresh: undefined }))
        .then(({ question, fresh }) => {
          if (question) {
            setAiPending(false)
            onReload()
            return
          }
          const attemptRecorded = recordAttemptOutcome('listing')
          void updatePortalLink(link.id, { instructionsAiError: '', instructionsAiConversationId: '' })
            .catch((recordErr: unknown) => {
              console.error('failed to clear instructions-AI conversation on portal link', link.id, recordErr)
            })
            .then(() => attemptRecorded)
            .then((outcome) => {
              const sampleUrl = followUp ? extractSampleJobUrl(followUp.reply, link.url) : null
              if (
                followUp !== undefined &&
                sampleUrl !== null &&
                shouldContinueWithJobDetail({
                  withDetail: true,
                  outcome,
                  questionPending: false,
                  hasDetailInstructions: fresh ? hasJobDetailInstructions(fresh) : false,
                  sampleUrl,
                })
              ) {
                continueWithJobDetail(followUp.conversationId, sampleUrl)
                setAiPending(false)
                return
              }
              closeAiLoginWindow()
              setAiPending(false)
              onReload()
            })
        })
      return
    }

    const text =
      err instanceof CoreApiError && (err.status === 401 || err.status === 403)
        ? 'Ask an admin to grant you access to AI conversations.'
        : err instanceof Error
          ? err.message
          : 'Failed to generate crawl instructions.'
    // Step 95: ask the backend how the attempt ended BEFORE showing the error.
    // The agent may have saved its instructions on the very round the budget
    // ran out, which ends the turn in an error although the run succeeded.
    // Only when the verdict is not 'succeeded' (or cannot be read) is the
    // error shown — and then immediately, regardless of whether the durable
    // write below succeeds: a network failure recording the error must never
    // leave the failure completely invisible.
    void recordAttemptOutcome('listing', text).then((outcome) => {
      if (savedDespiteError(outcome)) {
        setAiLocalError(undefined)
        void updatePortalLink(link.id, { instructionsAiError: '', instructionsAiConversationId: '' })
          .catch((recordErr: unknown) => {
            console.error('failed to clear instructions-AI conversation on portal link', link.id, recordErr)
          })
          .then(onReload)
        return
      }
      setAiLocalError(text)
      void updatePortalLink(link.id, { instructionsAiError: text, instructionsAiConversationId: '' })
        .catch((recordErr: unknown) => {
          // Recording the failure durably itself also failed — the
          // local error above still shows for this session; see
          // step 33's own Open Question 1 for the reliability limit
          // durable recording already has (e.g. across a reload).
          console.error('failed to record instructions-AI error on portal link', link.id, recordErr)
        })
        .then(onReload)
    })
  }

  // "Generate with AI" (step 33) — creates a conversation the same way
  // handleCrawlWithAI does, but hidden (never shown in the normal
  // conversation list) and instructed to write/update this link's own
  // crawl_instructions instead of running a crawl. Runs in the
  // background regardless of this tab staying open
  // (plan/ai/conversation/step-23-detach-turn-execution-from-request.md).
  // Opens the small floating chat widget immediately (step 60) so the
  // user can watch the AI work instead of only seeing a disabled
  // button — see this step's own design doc for why that widget needs
  // no changes of its own to support this.
  //
  // Step 103: for a link that has no listing instructions yet, the portal's other links'
  // instructions are tried on it first (no AI, no tokens). When they work the AI run is
  // skipped: they are saved and the usual pipeline carries on. When they do not, or the
  // trial cannot run, the AI run below starts as before. A run that follows the user's
  // "yes, this page has jobs" answer never tries them: those instructions were proven wrong.
  function handleGenerateInstructionsWithAI(options: { jobsConfirmed?: boolean } = {}) {
    if (aiPending || inheriting || !selectedPlatform) return
    if (shouldTryInherit(link) && !options.jobsConfirmed) {
      void onTryInherit().then((inherited) => {
        if (!inherited) startGenerateInstructionsWithAI(options)
      })
      return
    }
    startGenerateInstructionsWithAI(options)
  }

  function startGenerateInstructionsWithAI(options: { jobsConfirmed?: boolean } = {}) {
    if (aiPending || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    // Plan 163, step 3: a link with no job-detail instructions yet gets them in the same conversation, right after its listing
    // instructions are saved (see continueWithJobDetail); the AI is then also allowed the tool that saves them.
    const withDetail = !hasJobDetailInstructions(link)
    const tools = withDetail ? GENERATE_LISTING_WITH_DETAIL_TOOLS : GENERATE_LISTING_TOOLS
    let conversationId = ''
    setAiPending(true)
    setAiLocalError(undefined)

    const create = () =>
      createConversation({
        title: generateInstructionsConversationTitle(link),
        platformId,
        model,
        hidden: true,
        toolIterationLimit: GENERATE_TOOL_ITERATION_LIMIT,
        allowedTools: [...tools],
        compactToolResults: true,
      })
    // Plan 183: a link that needs a login. The conversation exists first (its id is the login window's id), the window opens for the person,
    // and only after "I am logged in" is the attempt counted and the AI started; before that nothing is counted, so cancelling is no failure.
    const head: Promise<Awaited<ReturnType<typeof createConversation>>> = link.requiresLogin
      ? startAfterLogin({
          create,
          open: (id) => openAiLoginWindow(id, link),
          windowOpened: (id) => {
            aiLoginWindowRef.current = id
          },
          waitForPerson: waitForPersonToLogin,
          recordStart: () => recordAttemptStart('listing', 'manual'),
          closeWindow: closeAiLoginWindow,
          removeConversation: deleteConversation,
          isCancel: (err) => err instanceof LoginWaitCancelled,
        })
      : // Counted before the conversation exists, so a failure to even
        // create it still shows up as a failed attempt (step 86). Only the
        // user's own click starts a listing run.
        recordAttemptStart('listing', 'manual').then(create)
    head
      .then((conversation) => {
        if (!restrictionHolds(conversation.allowedTools, tools)) throw new Error(GENERATE_RESTRICTION_NOT_APPLIED_MESSAGE)
        conversationId = conversation.id
        setAiConversationId(conversation.id)
        window.__cinqoToolBridge.openConversation(conversation.id)
        return updatePortalLink(link.id, { instructionsAiConversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one run, not the run itself.
          })
          .then(() => sendMessage(conversation.id, buildGenerateInstructionsMessage(link, { ...options, withJobDetail: withDetail })))
      })
      .then(
        (reply) => {
          finishGenerateInstructions(undefined, withDetail ? { conversationId, reply: reply.content } : undefined)
        },
        (err: unknown) => {
          if (err instanceof LoginGenerateAbandoned) {
            // The run never started (the person cancelled, the window could not be opened, ...): nothing was counted, so this is no failed
            // attempt; the window was closed and the empty conversation removed by startAfterLogin.
            abandonLoginGenerate(err)
            return
          }
          finishGenerateInstructions(err)
        },
      )
  }

  // plan 183: a run for a link that needs a login that ended before it started. Not a failure: no attempt was recorded.
  function abandonLoginGenerate(abandoned: LoginGenerateAbandoned) {
    setAiLoginWait(null)
    setAiPending(false)
    if (abandoned.reason === undefined) return // the person cancelled
    setAiLocalError(abandoned.reason instanceof Error ? abandoned.reason.message : 'Could not start the AI run.')
  }

  // handleCheckProgress never starts a new run — it only reopens the
  // widget onto the one already tracked in aiConversationId (set by
  // either handleGenerateInstructionsWithAI or
  // resumeGenerateInstructions above).
  function handleCheckProgress() {
    const conversationId = aiConversationId ?? link.instructionsAiConversationId
    if (conversationId) {
      window.__cinqoToolBridge.openConversation(conversationId)
    }
  }

  // finishGenerateJobDetailInstructions mirrors finishGenerateInstructions
  // above exactly, for the SEPARATE job-detail instructions document's
  // own AI generation — its own independent ai*/instructionsAi* state
  // and fields, never the listing instructions' own.
  function finishGenerateJobDetailInstructions(err?: unknown) {
    setJobDetailAiPending(false)
    setJobDetailAiConversationId(null)
    closeAiLoginWindow() // the run for a link that needs a login is over (plan 183)

    if (!err) {
      // jobDetailInstructionsAiRequested: false (step 67) — cleared
      // only on success, alongside the fields this call already
      // cleared: a failure leaves the flag set, so simply reopening the
      // page retries the automatic generation again (see
      // jobDetailAiRequestedRef's own effect above), with no manual
      // fallback button needed.
      const attemptRecorded = recordAttemptFinish('job_detail')
      void updatePortalLink(link.id, {
        jobDetailInstructionsAiError: '',
        jobDetailInstructionsAiConversationId: '',
        jobDetailInstructionsAiRequested: false,
      })
        .catch((recordErr: unknown) => {
          console.error('failed to clear job detail instructions-AI conversation on portal link', link.id, recordErr)
        })
        .then(() => attemptRecorded)
        .then(onReload)
      return
    }

    const text =
      err instanceof CoreApiError && (err.status === 401 || err.status === 403)
        ? 'Ask an admin to grant you access to AI conversations.'
        : err instanceof Error
          ? err.message
          : 'Failed to generate job detail crawl instructions.'
    // Step 95: as for the listing instructions above — the backend's verdict
    // decides, since a turn can end in an error after a successful save.
    void recordAttemptOutcome('job_detail', text).then((outcome) => {
      if (savedDespiteError(outcome)) {
        setJobDetailAiLocalError(undefined)
        void updatePortalLink(link.id, {
          jobDetailInstructionsAiError: '',
          jobDetailInstructionsAiConversationId: '',
          jobDetailInstructionsAiRequested: false,
        })
          .catch((recordErr: unknown) => {
            console.error('failed to clear job detail instructions-AI conversation on portal link', link.id, recordErr)
          })
          .then(onReload)
        return
      }
      setJobDetailAiLocalError(text)
      void updatePortalLink(link.id, { jobDetailInstructionsAiError: text, jobDetailInstructionsAiConversationId: '' })
        .catch((recordErr: unknown) => {
          console.error('failed to record job detail instructions-AI error on portal link', link.id, recordErr)
        })
        .then(onReload)
    })
  }

  // handleGenerateJobDetailInstructionsWithAI mirrors
  // handleGenerateInstructionsWithAI above exactly, but sends
  // buildGenerateJobDetailInstructionsMessage instead — writes/updates
  // job_detail_crawl_instructions, never crawl_instructions.
  function handleGenerateJobDetailInstructionsWithAI() {
    // an automatic start (no person to log in) is never allowed for a link that needs a login: the server refuses it too
    if (jobDetailAiPending || !selectedPlatform || link.requiresLogin) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setJobDetailAiPending(true)
    setJobDetailAiLocalError(undefined)

    // Only the automatic trigger (jobDetailAiRequestedRef's effect)
    // starts a job-detail run — there is no manual button — so this is
    // always counted as 'auto' (step 86).
    recordAttemptStart('job_detail', 'auto')
      .then(() =>
        createConversation({
          title: generateJobDetailInstructionsConversationTitle(link),
          platformId,
          model,
          hidden: true,
          toolIterationLimit: GENERATE_TOOL_ITERATION_LIMIT,
          allowedTools: [...GENERATE_JOB_DETAIL_TOOLS],
          compactToolResults: true,
        }),
      )
      .then((conversation) => {
        if (!restrictionHolds(conversation.allowedTools, GENERATE_JOB_DETAIL_TOOLS))
          throw new Error(GENERATE_RESTRICTION_NOT_APPLIED_MESSAGE)
        setJobDetailAiConversationId(conversation.id)
        window.__cinqoToolBridge.openConversation(conversation.id)
        return updatePortalLink(link.id, { jobDetailInstructionsAiConversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one run, not the run itself.
          })
          .then(() =>
            sendMessage(
              conversation.id,
              buildGenerateJobDetailInstructionsMessage(link, { linkCrawled: listingNeeds(engine, link.requiresLogin) === 'rule' }),
            ),
          )
      })
      .then(
        () => {
          finishGenerateJobDetailInstructions()
        },
        (err: unknown) => {
          finishGenerateJobDetailInstructions(err)
        },
      )
  }

  // continueWithJobDetail (plan 163, step 3) goes on in the conversation of a listing run whose instructions were just saved: its own
  // 'job_detail' attempt is counted (so the pills and the statistics stay truthful: a failure here is a failed job-detail attempt, never a
  // failed listing one, which already ended), the conversation is also tracked as this link's job-detail run (so "Check progress" and a
  // reload find it), and the follow-up message asks for the job-detail instructions from the first item of the AI's own last test crawl.
  function continueWithJobDetail(conversationId: string, sampleUrl: string) {
    setJobDetailAiPending(true)
    setJobDetailAiLocalError(undefined)
    setJobDetailAiConversationId(conversationId)
    recordAttemptStart('job_detail', 'manual')
      .then(() =>
        updatePortalLink(link.id, { jobDetailInstructionsAiConversationId: conversationId }).catch(() => {
          // Best-effort — a failure here only costs reload-resilience for this one run, not the run itself.
        }),
      )
      .then(() => sendMessage(conversationId, buildJobDetailFollowUpMessage(link, sampleUrl)))
      .then(
        () => {
          finishGenerateJobDetailInstructions()
        },
        (err: unknown) => {
          finishGenerateJobDetailInstructions(err)
        },
      )
  }

  // handleCheckJobDetailProgress mirrors handleCheckProgress above —
  // reopens the widget onto jobDetailAiConversationId, never
  // aiConversationId.
  function handleCheckJobDetailProgress() {
    const conversationId = jobDetailAiConversationId ?? link.jobDetailInstructionsAiConversationId
    if (conversationId) {
      window.__cinqoToolBridge.openConversation(conversationId)
    }
  }

  // Step 91 — the generation agent asked whether this page has jobs and is
  // waiting for the answer. Shown instead of the step 90 box (same
  // situation, one question).
  const jobsQuestion = pendingJobsQuestion(link)

  // Plan 163, step 4: a run counts as going on when this page carries it (aiPending) OR the link says so (its conversation id): a run the
  // run follower finishes, after a reload or from another page, shows here the same as one started on this page. A listing run that waits
  // for the user's answer is not running.
  const aiBusy = aiPending || (Boolean(link.instructionsAiConversationId) && !jobsQuestion)
  const jobDetailAiBusy = jobDetailAiPending || Boolean(link.jobDetailInstructionsAiConversationId)

  // Alert pills (step 86): attempts were made but this document still
  // doesn't exist. Computed from the server-side counters plus this
  // tab's own in-flight state.
  // Plan 190: only where the AI writes the job list (not under Katana, which uses a rule made by hand).
  const aiWritesListing = listingIsWrittenByAi(engine, link.requiresLogin)
  const hasListing = hasListingFor(link, engine)
  const listingAlertAttempts = aiWritesListing ? instructionAlertAttempts(link.instructionStats.listing, hasListing, aiBusy) : 0
  // Step 87 — once the AI has given up on the listing three times in a
  // row, the stronger "probably can't be crawled" pill replaces the
  // generic alert for the listing.
  const listingUncrawlable = aiWritesListing && isLikelyUncrawlable(link.instructionStats.listing, hasListing, aiBusy)
  const aiWritesJobDetail = jobDetailIsWrittenByAi(engine, link.requiresLogin)
  const jobDetailAlertAttempts = aiWritesJobDetail
    ? instructionAlertAttempts(link.instructionStats.jobDetail, Boolean(link.jobDetailCrawlInstructions), jobDetailAiBusy)
    : 0

  // "Yes" tells the agent in the same conversation that its approach was
  // wrong and lets it retry (the attempt stays open and ends with that
  // turn). "No" only records the answer: the page has no jobs.
  function handleAnswerJobsQuestion(answer: 'yes' | 'no') {
    const conversationId = link.instructionsAiConversationId
    if (answer === 'yes' && !conversationId) {
      onError('The conversation with the AI is no longer available. Use Generate to start again.')
      return
    }
    updatePortalLink(link.id, { jobsQuestionAnswer: answer })
      .then(() => {
        if (answer === 'no' || !conversationId) {
          return updatePortalLink(link.id, { instructionsAiConversationId: '' }).then(onReload)
        }
        setAiPending(true)
        setAiLocalError(undefined)
        setAiConversationId(conversationId)
        window.__cinqoToolBridge.openConversation(conversationId)
        onReload()
        void sendMessage(conversationId, buildJobsConfirmedMessage(link)).then(
          () => {
            finishGenerateInstructions()
          },
          (err: unknown) => {
            finishGenerateInstructions(err)
          },
        )
        return undefined
      })
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : String(err))
      })
  }

  // Step 90 — the latest listing crawl completed but found no jobs: what
  // is known about why, and the user's way to say "this page has no jobs".
  const emptyCrawl = emptyCrawlFor(link)
  const emptyDiagnosis = emptyCrawl ? diagnoseEmptyCrawl(emptyCrawl) : undefined

  function handleAcknowledgeEmptyCrawl() {
    updatePortalLink(link.id, { emptyCrawlAcknowledged: true })
      .then(onReload)
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleViewConversation() {
    window.__cinqoToolBridge.navigate('/conversations')
  }

  // Plan 190: "Job list" opens the rules under Katana (the link crawler, in its own modal) and the AI instructions under Rod (and for a
  // link that needs a login).
  // Plan 190: under Katana the job page is read by PICKING its data (JobDetailPickPanel), not by selectors typed by hand: the text box of
  // selectors belongs to Rod. A link that needs a login cannot use the picker, so it keeps the text box under every engine.
  const jobDetailPickOnly = listingNeeds(engine, link.requiresLogin) === 'rule'

  function openCrawlInstructionsModal() {
    if (listingNeeds(engine, link.requiresLogin) === 'rule') {
      setActiveInstructionsModal('rules')
      return
    }
    setInstructionsDraft(link.crawlInstructions ?? '')
    setActiveInstructionsModal('crawl')
  }

  function handleSaveCrawlInstructions() {
    updatePortalLink(link.id, { crawlInstructions: instructionsDraft })
      .then(() => {
        setActiveInstructionsModal(null)
        onReload()
      })
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : String(err))
      })
  }

  function openJobDetailInstructionsModal() {
    setJobDetailInstructionsDraft(link.jobDetailCrawlInstructions ?? '')
    setActiveInstructionsModal('jobDetail')
  }

  function handleSaveJobDetailCrawlInstructions() {
    updatePortalLink(link.id, { jobDetailCrawlInstructions: jobDetailInstructionsDraft })
      .then(() => {
        setActiveInstructionsModal(null)
        onReload()
      })
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : String(err))
      })
  }

  // handleDismissListingError/handleDismissJobDetailError (step 72) —
  // clear the same-session local copy immediately (so the box
  // disappears instantly, before the network round trip completes) and
  // durably record the dismissal against the error's own errorAt value
  // (never "now" — see step 71's own design doc for why exact equality
  // matters), falling back to "now" only in the narrow edge case where
  // a local error exists with no durable errorAt yet.
  function handleDismissListingError() {
    setAiLocalError(undefined)
    const dismissedAt = link.instructionsAiErrorAt ?? new Date().toISOString()
    void updatePortalLink(link.id, { instructionsAiErrorDismissedAt: dismissedAt }).then(onReload)
  }

  function handleDismissJobDetailError() {
    setJobDetailAiLocalError(undefined)
    const dismissedAt = link.jobDetailInstructionsAiErrorAt ?? new Date().toISOString()
    void updatePortalLink(link.id, { jobDetailInstructionsAiErrorDismissedAt: dismissedAt }).then(onReload)
  }

  return (
    <div className="mt-3">
      {/* Auto-discovery opt-out (step 84 follow-up) — always visible,
          outside the collapsible "Crawl instructions" accordion below,
          since it's a standing setting someone might come looking for
          without needing to also expand that section. The global
          on/off+interval (Crawler/AutoDiscoveryPanel) controls whether
          the mechanism runs at all; this is this ONE link's own
          override on top of that. */}
      <PipelineLine link={link} />
      {/* Step 88 — only for a link that has listing crawl instructions:
          auto-discovery crawls the listing with them, and the backend
          never auto-crawls a link without. Hiding this never changes the
          stored setting. */}
      {manualOnlyNote(link) !== null && <p className="mb-2 text-xs text-gray-600">{manualOnlyNote(link)}</p>}
      {canCrawlListing(link, engine) && !link.requiresLogin && (
        <div className="mb-2 flex flex-wrap items-center gap-2">
          <Switch
            checked={link.autoDiscoveryEnabled}
            disabled={autoDiscoveryPending}
            onChange={handleToggleAutoDiscovery}
            label="Auto-discovery"
            size="sm"
          />
          {/* This link's own interval override (step 85) — shown only
              while auto-discovery is actually on for this link; hidden,
              not just disabled, while it's off, since "how often" has no
              meaning at all until "whether" is yes. "Use global
              interval" (an empty string, the same "" = unselected
              convention this tool's other Dropdown call sites already
              use) clears the override entirely rather than sending a
              literal interval value. */}
          {link.autoDiscoveryEnabled && (
            <Dropdown
              options={[
                { value: AUTO_DISCOVERY_GLOBAL_VALUE, label: 'Use global interval' },
                ...AUTO_DISCOVERY_INTERVAL_OPTIONS.map((o) => ({ value: String(o.value), label: o.label })),
              ]}
              value={
                link.autoDiscoveryIntervalMinutes !== undefined ? String(link.autoDiscoveryIntervalMinutes) : AUTO_DISCOVERY_GLOBAL_VALUE
              }
              onChange={handleChangeAutoDiscoveryInterval}
            />
          )}
        </div>
      )}
      {/* Two small cards, each exactly half the row (grid-cols-2, not
          flex-wrap) so they always sit side by side at 50%/50% width
          regardless of either card's own content length — a flex row
          would let one card's shorter content shrink it below 50%. The
          single "Generate instructions" button (step 68) sits beside
          this grid, not inside either card — one control now generates
          the LISTING instructions only; job-detail instructions are
          entirely automatic (step 67's own event-driven pipeline), so
          neither card has its own manual trigger anymore. */}
      {/* Grouping section (step 74, later stripped of its own border/
          background/shadow per direct user request — now just a
          labeled, collapsible group): everything about "this link's
          own crawl instructions" (its error banners, both small cards,
          and the one button that generates the listing document) lives
          under one shared "Crawl instructions" accordion, distinct
          from the crawl-execution button rows and the status/log
          display that follow it below. See
          plan/ai/tools/career/step-74-crawl-instructions-grouping.md. */}
      <div className="py-3">
        {/* The shared AccordionItem (step 89, replacing a hand-rolled
            button + chevron + conditional) — closed by default once this
            link already has listing crawl instructions (nothing here
            needs the user's attention), open by default when it doesn't
            (exactly what does). See crawlInstructionsOpen's own doc
            comment above for why this is a one-time initial value, not
            a value kept in sync with link.crawlInstructions forever.
            The body stays mounted while closed. */}
        <AccordionItem
          variant="plain"
          isOpen={crawlInstructionsOpen}
          onToggle={() => {
            setCrawlInstructionsOpen((prev) => !prev)
          }}
          header={<h3 className="text-xs font-semibold tracking-wide text-gray-500 uppercase">Crawl instructions</h3>}
        >
          <div className="mt-2">
            {/* Two small cards, each exactly half the row (grid-cols-2, not
            flex-wrap) so they always sit side by side at 50%/50% width
            regardless of either card's own content length — a flex row
            would let one card's shorter content shrink it below 50%. The
            single "Generate instructions" button (step 68) sits beside
            this grid, not inside either card — one control now generates
            the LISTING instructions only; job-detail instructions are
            entirely automatic (step 67's own event-driven pipeline), so
            neither card has its own manual trigger anymore. */}
            {/* Error banners live OUTSIDE and ABOVE both cards (not nested
            inside whichever document happened to fail), each labeled
            with a pill naming which document failed — up to two can show
            at once, since the listing and job-detail generations fail
            independently. Gated on `aiLocalError || shouldShowError(...)`
            (not shouldShowError alone) to close a narrow timing window:
            the same-session local copy can be set slightly before its
            durable errorAt lands in a reloaded `link` prop — see
            plan/ai/tools/career/step-72-wire-infobox-into-crawlpanel.md's
            own "Open question". */}
            {(aiLocalError ?? shouldShowError(link.instructionsAiErrorAt, link.instructionsAiErrorDismissedAt)) && (
              <InfoBox
                variant="error"
                label="Job list crawl instructions"
                labelClassName="bg-blue-100 text-blue-800"
                message={
                  (aiLocalError ?? link.instructionsAiError ?? '') +
                  (!aiLocalError && link.instructionsAiErrorAt ? ` (${new Date(link.instructionsAiErrorAt).toLocaleString()})` : '')
                }
                onDismiss={handleDismissListingError}
              />
            )}
            {(jobDetailAiLocalError ??
              shouldShowError(link.jobDetailInstructionsAiErrorAt, link.jobDetailInstructionsAiErrorDismissedAt)) && (
              <InfoBox
                variant="error"
                label="Job detail crawl instructions"
                labelClassName="bg-purple-100 text-purple-800"
                message={
                  (jobDetailAiLocalError ?? link.jobDetailInstructionsAiError ?? '') +
                  (!jobDetailAiLocalError && link.jobDetailInstructionsAiErrorAt
                    ? ` (${new Date(link.jobDetailInstructionsAiErrorAt).toLocaleString()})`
                    : '')
                }
                onDismiss={handleDismissJobDetailError}
              />
            )}
            <div className="flex items-start gap-3">
              <div className="grid flex-1 grid-cols-2 gap-3">
                <div className="flex flex-col items-center justify-center rounded-lg border border-gray-200 bg-gray-50 p-2.5 text-center transition-colors hover:border-gray-300 hover:bg-gray-100/60">
                  <button
                    type="button"
                    onClick={openCrawlInstructionsModal}
                    title={
                      listingNeeds(engine, link.requiresLogin) === 'rule'
                        ? hasListing
                          ? 'Job list rules (Katana) — view/edit'
                          : 'Job list rules (Katana) — not set yet, click to add'
                        : hasListing
                          ? 'Job list crawl instructions — view/edit'
                          : 'Job list crawl instructions — not set yet, click to add'
                    }
                    className={`flex flex-col items-center justify-center gap-1 text-xs font-medium hover:underline ${hasListing ? 'text-gray-700' : 'text-gray-400'}`}
                  >
                    <CrawlInstructionsIcon />
                    Job list
                  </button>
                  {aiWritesListing && (
                    <InstructionAttemptStats
                      stats={link.instructionStats.listing}
                      alertAttempts={listingAlertAttempts}
                      uncrawlable={listingUncrawlable}
                    />
                  )}
                </div>
                <div className="flex flex-col items-center justify-center rounded-lg border border-gray-200 bg-gray-50 p-2.5 text-center transition-colors hover:border-gray-300 hover:bg-gray-100/60">
                  <button
                    type="button"
                    onClick={openJobDetailInstructionsModal}
                    title={
                      link.jobDetailCrawlInstructions
                        ? "Job detail crawl instructions — view/edit — how to extract the job-position-relevant text off a single job's own detail page (the page a listing's own job URL points to), separate from the listing crawl instructions."
                        : "Job detail crawl instructions — not set yet, generated automatically once this link's own listing has been crawled — how to extract the job-position-relevant text off a single job's own detail page (the page a listing's own job URL points to), separate from the listing crawl instructions."
                    }
                    className={`flex flex-col items-center justify-center gap-1 text-xs font-medium hover:underline ${link.jobDetailCrawlInstructions ? 'text-gray-700' : 'text-gray-400'}`}
                  >
                    <JobDetailInstructionsIcon />
                    Job details
                  </button>
                  {aiWritesJobDetail && (
                    <InstructionAttemptStats stats={link.instructionStats.jobDetail} alertAttempts={jobDetailAlertAttempts} />
                  )}
                  {/* Passive status only — no manual trigger anymore. This
                  only ever becomes true via jobDetailAiRequestedRef's own
                  automatic effect (step 67), never a direct click. */}
                  {jobDetailAiBusy && (
                    <button
                      type="button"
                      onClick={handleCheckJobDetailProgress}
                      title="Open the chat window to watch the AI work on this link's job detail crawl instructions"
                      className="mt-1 inline-flex items-center gap-1 text-xs text-gray-500 underline hover:text-gray-700"
                    >
                      <RobotIcon />
                      Generating...
                    </button>
                  )}
                </div>
              </div>
              {aiWritesListing && (
                <button
                  type="button"
                  onClick={
                    aiBusy
                      ? handleCheckProgress
                      : () => {
                          handleGenerateInstructionsWithAI()
                        }
                  }
                  disabled={
                    !hasPlatforms ||
                    inheriting ||
                    (aiBusy && !(aiConversationId ?? link.instructionsAiConversationId)) ||
                    Boolean(jobsQuestion)
                  }
                  title={
                    !hasPlatforms
                      ? 'No AI platform configured — add one on the Platforms page first'
                      : jobsQuestion
                        ? 'Answer the question below first'
                        : inheriting
                          ? "Trying the instructions of this platform's other links first…"
                          : aiBusy
                            ? "Open the chat window to watch the AI work on this link's crawl instructions"
                            : 'Let the AI inspect this page and write (or update) its listing crawl instructions for you — once saved, this link is crawled and its job-detail instructions are generated automatically.'
                  }
                  className="flex shrink-0 flex-col items-center justify-center gap-1 self-stretch rounded-lg border border-gray-200 bg-gray-50 px-3 text-xs font-medium text-gray-700 transition-colors hover:border-gray-300 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {aiBusy ? (
                    <>
                      <RobotIcon className="h-5 w-5" />
                      Generating...
                    </>
                  ) : (
                    <>
                      <RobotIcon className="h-5 w-5" />
                      Generate
                    </>
                  )}
                </button>
              )}
            </div>
            {aiLoginWait !== null ? (
              <div role="status" className="mt-2 rounded-md border border-sky-300 bg-sky-50 px-2 py-1.5 text-xs text-sky-900">
                <p className="font-medium">{AI_LOGIN_WAIT_TEXT}</p>
                <p className="mt-0.5 text-sky-800">You type your login on the site itself; nothing of it is kept here.</p>
                <div className="mt-1.5 flex gap-2">
                  <button
                    type="button"
                    disabled={isConfirmingLogin}
                    onClick={handleConfirmAiLogin}
                    className="rounded-md bg-sky-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-sky-800 disabled:opacity-50"
                  >
                    I am logged in
                  </button>
                  <button
                    type="button"
                    onClick={handleCancelAiLogin}
                    className="rounded-md border border-sky-300 bg-white px-2.5 py-1 text-xs text-sky-900 hover:bg-sky-100"
                  >
                    Cancel
                  </button>
                </div>
                {loginConfirmError && (
                  <p role="alert" className="mt-1 text-red-700">
                    {loginConfirmError}
                  </p>
                )}
              </div>
            ) : (
              generateLoginNote(link) !== null && (
                <p role="status" className="mt-2 text-xs text-gray-600">
                  {generateLoginNote(link)}
                </p>
              )
            )}
            {aiWritesListing && (
              <StartFreshButton
                link={link}
                tabBusy={{ listingGenerating: aiBusy, jobDetailGenerating: jobDetailAiBusy, inheriting }}
                canGenerate={hasPlatforms && selectedPlatform !== null}
                onGenerate={() => {
                  handleGenerateInstructionsWithAI()
                }}
                onReload={onReload}
              />
            )}
          </div>
        </AccordionItem>
      </div>
      {/* Step 103: the trial of the portal's other instructions, while it runs and what it said. */}
      {aiWritesListing && inheriting && (
        <p role="status" className="mt-2 text-xs text-gray-600">
          Trying the instructions of this platform's other links on this page…
        </p>
      )}
      {aiWritesListing && !inheriting && inheritNote && (
        <p
          role="status"
          className={`mt-2 text-xs ${
            inheritNote.kind === 'success' || inheritNote.kind === 'partial'
              ? 'text-green-700'
              : inheritNote.kind === 'error'
                ? 'text-amber-700'
                : 'text-gray-600'
          }`}
        >
          {inheritNote.message}
        </p>
      )}
      {/* Step 93: shown where Generate is the next action (no listing instructions
          yet), only while the random free router is the selected model. */}
      {aiWritesListing && !link.crawlInstructions && isFreeRouter(selectedPlatform, selectedModel) && (
        <p role="note" className="mt-2 text-xs text-amber-700">
          {FREE_ROUTER_HINT}
        </p>
      )}
      {aiWritesListing && jobsQuestion && (
        <div className="mt-2">
          <InfoBox
            variant="warning"
            label="Question for you"
            message={JOBS_QUESTION_TITLE}
            details={
              <>
                {jobsQuestion.note && <p>The AI says: {jobsQuestion.note}</p>}
                <p className="mt-1">
                  {JOBS_QUESTION_YES_HINT} {JOBS_QUESTION_NO_HINT}
                </p>
              </>
            }
            actions={
              <>
                <a
                  href={link.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs text-orange-800 hover:bg-orange-100"
                >
                  Open the page
                </a>
                <button
                  type="button"
                  onClick={() => {
                    handleAnswerJobsQuestion('yes')
                  }}
                  className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs font-medium text-orange-800 hover:bg-orange-100"
                >
                  Yes, it has jobs
                </button>
                <button
                  type="button"
                  onClick={() => {
                    handleAnswerJobsQuestion('no')
                  }}
                  className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs font-medium text-orange-800 hover:bg-orange-100"
                >
                  No, it has none
                </button>
              </>
            }
          />
        </div>
      )}
      {aiWritesListing && emptyCrawl && emptyDiagnosis && !jobsQuestion && (
        <div className="mt-2">
          <InfoBox
            variant="warning"
            label="Crawled 0 jobs"
            message={emptyDiagnosis.text}
            details={
              <>
                {emptyCrawlFacts(emptyCrawl)}
                {!emptyDiagnosis.conclusive && <p className="mt-1 font-medium">Does this page list jobs?</p>}
              </>
            }
            actions={
              <>
                <a
                  href={link.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs text-orange-800 hover:bg-orange-100"
                >
                  Open the page
                </a>
                {emptyDiagnosis.conclusive ? (
                  <button
                    type="button"
                    onClick={() => {
                      handleGenerateInstructionsWithAI()
                    }}
                    disabled={!hasPlatforms || aiBusy}
                    title={!hasPlatforms ? 'No AI platform configured — add one on the Platforms page first' : undefined}
                    className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs text-orange-800 hover:bg-orange-100 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    Regenerate instructions
                  </button>
                ) : (
                  <>
                    {/* Step 91: the crawler cannot tell "no jobs" from "wrong
                        selectors", so the user is asked. Yes regenerates with
                        that fact in the prompt; No is the old "mark as expected". */}
                    <button
                      type="button"
                      onClick={() => {
                        handleGenerateInstructionsWithAI({ jobsConfirmed: true })
                      }}
                      disabled={!hasPlatforms || aiBusy}
                      title={
                        hasPlatforms
                          ? 'The instructions are wrong: the AI is told the page has jobs and writes new ones.'
                          : 'No AI platform configured — add one on the Platforms page first'
                      }
                      className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs font-medium text-orange-800 hover:bg-orange-100 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      Yes, it has jobs
                    </button>
                    <button
                      type="button"
                      onClick={handleAcknowledgeEmptyCrawl}
                      title="Nothing is wrong. This hides the notice until a crawl finds a job."
                      className="rounded-md border border-orange-200 bg-white px-2.5 py-1 text-xs font-medium text-orange-800 hover:bg-orange-100"
                    >
                      No, it has none
                    </button>
                  </>
                )}
              </>
            }
          />
        </div>
      )}
      {canCrawlListing(link, engine) && (
        <div className="mt-1.5">
          <div className="flex flex-wrap gap-2">
            {run?.status === 'running' && run.kind === 'listing' ? (
              <>
                <button
                  type="button"
                  onClick={handleStopCrawlNow}
                  disabled={isStoppingCrawl}
                  title="Actually stop this crawl — interrupts it on the server (both Career's own goroutine and the browser tool's in-flight work), not just this tab's own view of it."
                  className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <StopIcon />
                  {isStoppingCrawl ? 'Stopping…' : 'Stop crawl'}
                </button>
                <button
                  type="button"
                  onClick={handleCancelCrawlNow}
                  title="Stop watching this crawl in this tab — it keeps running on the server regardless, and reopening this page later will show its latest status. See plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md."
                  className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                >
                  <PlayIcon />
                  Stop watching
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={handleCrawlNow}
                disabled={isBusy()}
                title="Deterministic — no AI, no platform needed. Runs on the server, so it keeps going even if you leave this page. Requires fields labeled title/url (and optionally company/location/description/postedAt) in this link's crawl instructions. If Cloudflare blocks it, this can take up to about 30 minutes while the browser tool waits for it to clear."
                className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <PlayIcon />
                Crawl now
              </button>
            )}
            <button
              type="button"
              onClick={handleCrawlWithAI}
              disabled={isBusy() || !hasPlatforms}
              title={
                !hasPlatforms
                  ? 'No AI platform configured — add one on the Platforms page first'
                  : 'Lets the AI drive the crawl itself — tolerates instructions that don’t fit "Crawl now"’s fixed fields, but needs a configured AI platform.'
              }
              className="hidden flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <PlayIcon />
              {isCrawling ? 'Crawling…' : 'Crawl with AI'}
            </button>
          </div>
          {crawlResult && (
            <p className={`mt-1 text-xs ${crawlResult.ok ? 'text-green-700' : 'text-red-700'}`}>
              {crawlResult.text}
              {crawlResult.ok && crawlResult.conversationId && (
                <>
                  {' — '}
                  <button type="button" onClick={handleViewConversation} className="underline hover:text-green-900">
                    View conversation
                  </button>
                </>
              )}
            </p>
          )}
        </div>
      )}
      {link.jobDetailCrawlInstructions && run?.status === 'running' && run.kind === 'job_detail' && (
        <div className="mt-1.5">
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={handleStopCrawlNow}
              disabled={isStoppingCrawl}
              title="Actually stop this crawl — interrupts it on the server (both Career's own goroutine and the browser tool's in-flight work), not just this tab's own view of it."
              className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <StopIcon />
              {isStoppingCrawl ? 'Stopping…' : 'Stop crawl'}
            </button>
            <button
              type="button"
              onClick={handleCancelCrawlNow}
              title="Stop watching this crawl in this tab — it keeps running on the server regardless, and reopening this page later will show its latest status."
              className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
            >
              <PlayIcon />
              Stop watching
            </button>
          </div>
        </div>
      )}
      {/* Shared live/terminal state for whichever crawl (listing or job
          detail — run.kind) is currently tracked for this link
          (steps 37/38/40; step XX made this generic over both kinds) —
          crawl_runs_one_running_idx guarantees only one of either kind
          can ever be active at a time, so one shared display is always
          unambiguous. Shown for every status, including while running:
          the backend tracks a real, fine-grained phase per step (step
          39, sourced from browser's own live status — step 31) and
          this instance polls it every 3s — surfacing the current phase
          live, plus the full log on demand, is what "the frontend
          shows the status and the logs" concretely means (not just on
          a terminal outcome). awaiting_human_challenge gets its own
          visually distinct callout — this is the one phase where the
          user, not the system, is the blocker. */}
      {run && (
        <div className="mt-1.5">
          <p className="text-xs font-medium text-gray-700">{run.kind === 'job_detail' ? 'Job detail crawl' : 'Listing crawl'}:</p>
          {run.status === 'running' &&
            (isWaitingForLogin(run) ? (
              <div className="rounded-md border border-sky-300 bg-sky-50 px-2 py-1.5 text-xs text-sky-900">
                <p className="font-medium">A window is open at the login page. Log in there, then press &quot;I am logged in&quot;.</p>
                <p className="mt-0.5 text-sky-800">You type your login on the site itself; nothing of it is kept here.</p>
                <div className="mt-1.5 flex gap-2">
                  <button
                    type="button"
                    disabled={isConfirmingLogin}
                    onClick={handleConfirmLogin}
                    className="rounded-md bg-sky-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-sky-800 disabled:opacity-50"
                  >
                    I am logged in
                  </button>
                  <button
                    type="button"
                    disabled={isStoppingCrawl}
                    onClick={handleStopCrawlNow}
                    className="rounded-md border border-sky-300 bg-white px-2.5 py-1 text-xs text-sky-900 hover:bg-sky-100 disabled:opacity-50"
                  >
                    Cancel
                  </button>
                </div>
                {loginConfirmError && (
                  <p role="alert" className="mt-1 text-red-700">
                    {loginConfirmError}
                  </p>
                )}
              </div>
            ) : run.phase === 'awaiting_human_challenge' ? (
              <p className="flex items-center gap-1.5 rounded-md border border-amber-300 bg-amber-50 px-2 py-1 text-xs font-medium text-amber-800">
                <AlertIcon />
                Waiting for you to solve a Cloudflare challenge in the browser window
              </p>
            ) : (
              <p className="text-xs text-gray-500">{phaseLabel(run.phase) ?? latestCrawlLogMessage(run.log) ?? 'Crawl in progress…'}</p>
            ))}
          {run.status === 'completed' && <p className="text-xs text-green-700">{run.resultSummary}</p>}
          {run.status === 'failed' && <p className="text-xs text-red-700">{run.errorMessage}</p>}
          <button
            type="button"
            onClick={() => {
              setLogExpanded((prev) => !prev)
            }}
            title="Show every logged step of this crawl run, from start to its current or final status."
            className="mt-1 flex items-center gap-1 rounded-md border border-gray-200 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-50"
          >
            <LogIcon />
            {logExpanded ? 'Hide log' : 'View log'}
          </button>
          {logExpanded && (
            <pre className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap rounded-md bg-gray-900 p-2 text-xs text-gray-100">
              {run.log.length > 0 ? run.log.join('\n') : '(no log entries yet)'}
            </pre>
          )}
        </div>
      )}
      <Modal
        open={activeInstructionsModal !== null}
        title={
          activeInstructionsModal === 'jobDetail'
            ? jobDetailPickOnly
              ? 'Job details (Katana)'
              : 'Job detail crawl instructions'
            : activeInstructionsModal === 'rules'
              ? 'Job list rules (Katana)'
              : 'Job list crawl instructions'
        }
        size={activeInstructionsModal === 'jobDetail' || activeInstructionsModal === 'rules' ? 'wide' : 'default'}
        onClose={() => {
          setActiveInstructionsModal(null)
        }}
      >
        {activeInstructionsModal === 'rules' && <LinkCrawlPanel link={link} onReload={onReload} />}
        {activeInstructionsModal === 'crawl' && (
          <>
            <textarea
              value={instructionsDraft}
              onChange={(e) => {
                setInstructionsDraft(e.target.value)
              }}
              placeholder={'fields:\n  - label: title\n    selector: h1\npagination:\n  nextSelector: a.next-page\n  maxPages: 5'}
              rows={10}
              spellCheck={false}
              className="w-full rounded-md border border-gray-300 px-2 py-1.5 font-mono text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
            />
            <div className="mt-3 flex gap-2">
              <button
                type="button"
                onClick={handleSaveCrawlInstructions}
                className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
              >
                Save
              </button>
              <button
                type="button"
                onClick={() => {
                  setActiveInstructionsModal(null)
                }}
                className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
              >
                Cancel
              </button>
            </div>
          </>
        )}
        {activeInstructionsModal === 'jobDetail' && (
          <>
            {!link.requiresLogin && (
              <JobDetailPickPanel
                link={link}
                onSaved={(yaml) => {
                  setJobDetailInstructionsDraft(yaml)
                  onReload()
                }}
              />
            )}
            {!jobDetailPickOnly && (
              <>
                <textarea
                  value={jobDetailInstructionsDraft}
                  onChange={(e) => {
                    setJobDetailInstructionsDraft(e.target.value)
                  }}
                  placeholder={'fields:\n  - label: description\n    selector: ".job-description, .job-posting-body"'}
                  rows={10}
                  spellCheck={false}
                  className="w-full rounded-md border border-gray-300 px-2 py-1.5 font-mono text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <div className="mt-3 flex gap-2">
                  <button
                    type="button"
                    onClick={handleSaveJobDetailCrawlInstructions}
                    className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setActiveInstructionsModal(null)
                    }}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
                  >
                    Cancel
                  </button>
                </div>
              </>
            )}
          </>
        )}
      </Modal>
    </div>
  )
}
