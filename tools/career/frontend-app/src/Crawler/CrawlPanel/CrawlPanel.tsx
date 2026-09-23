import { useEffect, useRef, useState } from 'react'
import { updatePortalLink, type PortalLink } from '../../api'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, sendMessage, awaitTurnCompletion, fetchTurnStatus } from '../../Cinqo/Conversation/conversation'
import { buildCrawlMessage, crawlConversationTitle } from '../crawl'
import {
  buildGenerateInstructionsMessage,
  generateInstructionsConversationTitle,
  buildGenerateJobDetailInstructionsMessage,
  generateJobDetailInstructionsConversationTitle,
} from '../generateInstructions'
import { startCrawlNow, startCrawlJobDetailsNow, stopCrawlNow, fetchActiveCrawlRun, type CrawlRun } from '../crawlNow'
import { phaseLabel, latestCrawlLogMessage } from '../crawlPhase'
import {
  PlayIcon,
  StopIcon,
  LogIcon,
  AlertIcon,
  RobotIcon,
  CrawlInstructionsIcon,
  JobDetailInstructionsIcon,
} from '../../Shared/Icons/icons'
import { Typewriter } from '../../Shared/Typewriter/Typewriter'
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
  onReload,
  onError,
}: {
  link: PortalLink
  selectedPlatform: Platform | null
  selectedModel: string | null
  hasPlatforms: boolean
  onReload: () => void
  onError: (message: string) => void
}) {
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
  const [aiPending, setAiPending] = useState(false)
  const [aiLocalError, setAiLocalError] = useState<string | undefined>(undefined)
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
  const [activeInstructionsModal, setActiveInstructionsModal] = useState<'crawl' | 'jobDetail' | null>(null)
  const [instructionsDraft, setInstructionsDraft] = useState('')

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
  const [jobDetailAiPending, setJobDetailAiPending] = useState(false)
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

  // Same reload-resilience as the "Crawl now" resume effect above, for
  // "Generate with AI" (step 60): link.instructionsAiConversationId is
  // the durable trace of a still-in-flight run surviving a page
  // reload/reopen — resumedAiRef guards it the same way resumedRef
  // does, for the same reason (an unrelated reload elsewhere on the
  // page must never reopen a check this instance's own finish handling
  // already cleared). See
  // plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
  const resumedAiRef = useRef(false)
  useEffect(() => {
    if (link.instructionsAiConversationId && !resumedAiRef.current) {
      resumedAiRef.current = true
      void resumeGenerateInstructions(link.instructionsAiConversationId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.instructionsAiConversationId])

  // Same reload-resilience as the listing "Generate with AI" resume
  // effect above, for the SEPARATE job-detail instructions' own
  // "Generate with AI" — link.jobDetailInstructionsAiConversationId is
  // that run's own independent durable trace.
  const resumedJobDetailAiRef = useRef(false)
  useEffect(() => {
    if (link.jobDetailInstructionsAiConversationId && !resumedJobDetailAiRef.current) {
      resumedJobDetailAiRef.current = true
      void resumeGenerateJobDetailInstructions(link.jobDetailInstructionsAiConversationId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.jobDetailInstructionsAiConversationId])

  // listingCrawlRequestedRef consumes link.listingCrawlRequested (step
  // 67) — a Domain Events listener (Career's own backend) sets this
  // flag once the AI has written fresh listing crawl instructions,
  // since the listener itself has no live user session to start a
  // crawl with. This effect performs that crawl under THIS tab's own
  // live session instead, the next time this link is rendered with the
  // flag set. See
  // plan/ai/tools/career/step-67-frontend-flag-consumption.md.
  const listingCrawlRequestedRef = useRef(false)
  useEffect(() => {
    if (link.listingCrawlRequested && !listingCrawlRequestedRef.current) {
      listingCrawlRequestedRef.current = true
      void updatePortalLink(link.id, { listingCrawlRequested: false }).catch(() => {
        // Best-effort — if this clear itself fails, the flag simply
        // fires again next reload, which is harmless: handleCrawlNow's
        // own isBusy() guard already no-ops a duplicate trigger.
      })
      handleCrawlNow()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.listingCrawlRequested])

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
  useEffect(() => {
    if (link.jobDetailInstructionsAiRequested && !link.jobDetailCrawlInstructions && !jobDetailAiRequestedRef.current) {
      jobDetailAiRequestedRef.current = true
      handleGenerateJobDetailInstructionsWithAI()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link.jobDetailInstructionsAiRequested, link.jobDetailCrawlInstructions])

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

  // "Crawl job details now" mirrors handleCrawlNow above exactly — the
  // SEPARATE deterministic mechanism (kind 'job_detail') that visits
  // every already-saved job's own detail page for this link instead of
  // the listing page. Shares `run`/`isBusy()`/watchCrawlRun with the
  // listing mechanism (crawl_runs_one_running_idx already guarantees
  // only one of either kind can be active for this link at a time), so
  // no separate run-tracking state is needed here.
  function handleCrawlJobDetailsNow() {
    if (isBusy()) return
    setCrawlResult(null)
    startCrawlJobDetailsNow(link.id)
      .then((started) => {
        setRun({
          crawlRunId: started.crawlRunId,
          kind: 'job_detail',
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
        setCrawlResult({ ok: false, text: err instanceof Error ? err.message : 'Failed to start job detail crawl.' })
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

  // finishGenerateInstructions is the shared "a generate-instructions
  // run just ended" handling (step 60) — used by both a fresh run
  // (handleGenerateInstructionsWithAI below) and a resumed one
  // (resumeGenerateInstructions), so a run that finishes while this
  // tab was away is handled identically to one that finishes while
  // it's open. err is the failure from awaitTurnCompletion, if any;
  // undefined means success.
  function finishGenerateInstructions(err?: unknown) {
    setAiPending(false)
    setAiConversationId(null)

    if (!err) {
      void updatePortalLink(link.id, { instructionsAiError: '', instructionsAiConversationId: '' })
        .catch((recordErr: unknown) => {
          console.error('failed to clear instructions-AI conversation on portal link', link.id, recordErr)
        })
        .then(onReload)
      return
    }

    const text =
      err instanceof CoreApiError && (err.status === 401 || err.status === 403)
        ? 'Ask an admin to grant you access to AI conversations.'
        : err instanceof Error
          ? err.message
          : 'Failed to generate crawl instructions.'
    // Shown immediately, regardless of whether the durable write below
    // succeeds — a network failure recording the error must never
    // leave the failure completely invisible.
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
  }

  // resumeGenerateInstructions picks a still-in-flight run back up
  // after a reload/reopen — link.instructionsAiConversationId is the
  // durable trace a fresh run's own handleGenerateInstructionsWithAI
  // wrote below. Checks whether it's actually still running before
  // committing to "Check Progress - AI": a run that finished while
  // this tab was away is reconciled immediately via the same finish
  // path a run finishing live already uses, instead of leaving the
  // button stuck on "Check Progress - AI" forever. A status-check
  // failure (network error, not "no turn found") is treated as "still
  // running" — optimistic, since silently reverting to "Generate with
  // AI" here would lose the durable conversation id for no reason; see
  // this step's own design doc "Open questions".
  async function resumeGenerateInstructions(conversationId: string) {
    setAiConversationId(conversationId)
    let status
    try {
      status = await fetchTurnStatus(conversationId)
    } catch {
      status = 'running' as const
    }
    if (status !== 'running') {
      awaitTurnCompletion(conversationId).then(
        () => {
          finishGenerateInstructions()
        },
        (err: unknown) => {
          finishGenerateInstructions(err)
        },
      )
      return
    }
    setAiPending(true)
    awaitTurnCompletion(conversationId).then(
      () => {
        finishGenerateInstructions()
      },
      (err: unknown) => {
        finishGenerateInstructions(err)
      },
    )
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
  function handleGenerateInstructionsWithAI() {
    if (aiPending || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setAiPending(true)
    setAiLocalError(undefined)

    createConversation({ title: generateInstructionsConversationTitle(link), platformId, model, hidden: true })
      .then((conversation) => {
        setAiConversationId(conversation.id)
        window.__cinqoToolBridge.openConversation(conversation.id)
        return updatePortalLink(link.id, { instructionsAiConversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one run, not the run itself.
          })
          .then(() => sendMessage(conversation.id, buildGenerateInstructionsMessage(link)))
      })
      .then(
        () => {
          finishGenerateInstructions()
        },
        (err: unknown) => {
          finishGenerateInstructions(err)
        },
      )
  }

  // handleCheckProgress never starts a new run — it only reopens the
  // widget onto the one already tracked in aiConversationId (set by
  // either handleGenerateInstructionsWithAI or
  // resumeGenerateInstructions above).
  function handleCheckProgress() {
    if (aiConversationId) {
      window.__cinqoToolBridge.openConversation(aiConversationId)
    }
  }

  // finishGenerateJobDetailInstructions mirrors finishGenerateInstructions
  // above exactly, for the SEPARATE job-detail instructions document's
  // own AI generation — its own independent ai*/instructionsAi* state
  // and fields, never the listing instructions' own.
  function finishGenerateJobDetailInstructions(err?: unknown) {
    setJobDetailAiPending(false)
    setJobDetailAiConversationId(null)

    if (!err) {
      // jobDetailInstructionsAiRequested: false (step 67) — cleared
      // only on success, alongside the fields this call already
      // cleared: a failure leaves the flag set, so simply reopening the
      // page retries the automatic generation again (see
      // jobDetailAiRequestedRef's own effect above), with no manual
      // fallback button needed.
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

    const text =
      err instanceof CoreApiError && (err.status === 401 || err.status === 403)
        ? 'Ask an admin to grant you access to AI conversations.'
        : err instanceof Error
          ? err.message
          : 'Failed to generate job detail crawl instructions.'
    setJobDetailAiLocalError(text)
    void updatePortalLink(link.id, { jobDetailInstructionsAiError: text, jobDetailInstructionsAiConversationId: '' })
      .catch((recordErr: unknown) => {
        console.error('failed to record job detail instructions-AI error on portal link', link.id, recordErr)
      })
      .then(onReload)
  }

  // resumeGenerateJobDetailInstructions mirrors resumeGenerateInstructions
  // above exactly, for the SEPARATE job-detail instructions document.
  async function resumeGenerateJobDetailInstructions(conversationId: string) {
    setJobDetailAiConversationId(conversationId)
    let status
    try {
      status = await fetchTurnStatus(conversationId)
    } catch {
      status = 'running' as const
    }
    if (status !== 'running') {
      awaitTurnCompletion(conversationId).then(
        () => {
          finishGenerateJobDetailInstructions()
        },
        (err: unknown) => {
          finishGenerateJobDetailInstructions(err)
        },
      )
      return
    }
    setJobDetailAiPending(true)
    awaitTurnCompletion(conversationId).then(
      () => {
        finishGenerateJobDetailInstructions()
      },
      (err: unknown) => {
        finishGenerateJobDetailInstructions(err)
      },
    )
  }

  // handleGenerateJobDetailInstructionsWithAI mirrors
  // handleGenerateInstructionsWithAI above exactly, but sends
  // buildGenerateJobDetailInstructionsMessage instead — writes/updates
  // job_detail_crawl_instructions, never crawl_instructions.
  function handleGenerateJobDetailInstructionsWithAI() {
    if (jobDetailAiPending || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setJobDetailAiPending(true)
    setJobDetailAiLocalError(undefined)

    createConversation({ title: generateJobDetailInstructionsConversationTitle(link), platformId, model, hidden: true })
      .then((conversation) => {
        setJobDetailAiConversationId(conversation.id)
        window.__cinqoToolBridge.openConversation(conversation.id)
        return updatePortalLink(link.id, { jobDetailInstructionsAiConversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one run, not the run itself.
          })
          .then(() => sendMessage(conversation.id, buildGenerateJobDetailInstructionsMessage(link)))
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

  // handleCheckJobDetailProgress mirrors handleCheckProgress above —
  // reopens the widget onto jobDetailAiConversationId, never
  // aiConversationId.
  function handleCheckJobDetailProgress() {
    if (jobDetailAiConversationId) {
      window.__cinqoToolBridge.openConversation(jobDetailAiConversationId)
    }
  }

  function handleViewConversation() {
    window.__cinqoToolBridge.navigate('/conversations')
  }

  function openCrawlInstructionsModal() {
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
    <div className="mt-1.5">
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
          label="Crawl instructions"
          message={
            (aiLocalError ?? link.instructionsAiError ?? '') +
            (!aiLocalError && link.instructionsAiErrorAt ? ` (${new Date(link.instructionsAiErrorAt).toLocaleString()})` : '')
          }
          onDismiss={handleDismissListingError}
        />
      )}
      {(jobDetailAiLocalError ?? shouldShowError(link.jobDetailInstructionsAiErrorAt, link.jobDetailInstructionsAiErrorDismissedAt)) && (
        <InfoBox
          variant="error"
          label="Job detail crawl instructions"
          message={
            (jobDetailAiLocalError ?? link.jobDetailInstructionsAiError ?? '') +
            (!jobDetailAiLocalError && link.jobDetailInstructionsAiErrorAt
              ? ` (${new Date(link.jobDetailInstructionsAiErrorAt).toLocaleString()})`
              : '')
          }
          onDismiss={handleDismissJobDetailError}
        />
      )}
      <div className="flex items-start gap-2">
        <div className="grid flex-1 grid-cols-2 gap-2">
          <div className="rounded-lg border border-gray-200 bg-gray-50 p-2">
            <button
              type="button"
              onClick={openCrawlInstructionsModal}
              title={link.crawlInstructions ? 'Crawl instructions — view/edit' : 'Crawl instructions — not set yet, click to add'}
              className={`flex items-center gap-1.5 text-xs font-medium hover:underline ${link.crawlInstructions ? 'text-gray-700' : 'text-gray-400'}`}
            >
              <CrawlInstructionsIcon />
              Crawl instructions
            </button>
          </div>
          <div className="rounded-lg border border-gray-200 bg-gray-50 p-2">
            <button
              type="button"
              onClick={openJobDetailInstructionsModal}
              title={
                link.jobDetailCrawlInstructions
                  ? "Job detail crawl instructions — view/edit — how to extract the job-position-relevant text off a single job's own detail page (the page a listing's own job URL points to), separate from the listing crawl instructions."
                  : "Job detail crawl instructions — not set yet, generated automatically once this link's own listing has been crawled — how to extract the job-position-relevant text off a single job's own detail page (the page a listing's own job URL points to), separate from the listing crawl instructions."
              }
              className={`flex items-center gap-1.5 text-xs font-medium hover:underline ${link.jobDetailCrawlInstructions ? 'text-gray-700' : 'text-gray-400'}`}
            >
              <JobDetailInstructionsIcon />
              Job detail crawl instructions
            </button>
            {/* Passive status only — no manual trigger anymore. This
                only ever becomes true via jobDetailAiRequestedRef's own
                automatic effect (step 67), never a direct click. */}
            {jobDetailAiPending && (
              <button
                type="button"
                onClick={handleCheckJobDetailProgress}
                title="Open the chat window to watch the AI work on this link's job detail crawl instructions"
                className="mt-1 inline-flex items-center gap-1 text-xs text-gray-500 underline hover:text-gray-700"
              >
                <span className="inline-flex items-center gap-1 [animation:robot-bob_1.6s_ease-in-out_infinite]">
                  <RobotIcon />
                  <Typewriter text="Check Progress - AI" />
                </span>
              </button>
            )}
          </div>
        </div>
        <button
          type="button"
          onClick={aiPending ? handleCheckProgress : handleGenerateInstructionsWithAI}
          disabled={!hasPlatforms || (aiPending && !aiConversationId)}
          title={
            !hasPlatforms
              ? 'No AI platform configured — add one on the Platforms page first'
              : aiPending
                ? "Open the chat window to watch the AI work on this link's crawl instructions"
                : 'Let the AI inspect this page and write (or update) its listing crawl instructions for you — once saved, this link is crawled and its job-detail instructions are generated automatically.'
          }
          className="flex shrink-0 items-center gap-1 self-stretch rounded-lg border border-gray-200 bg-gray-50 px-3 text-xs font-medium text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {aiPending ? (
            <span className="inline-flex items-center gap-1 [animation:robot-bob_1.6s_ease-in-out_infinite]">
              <RobotIcon />
              <Typewriter text="Check Progress - AI" />
            </span>
          ) : (
            <>
              <RobotIcon />
              Generate instructions
            </>
          )}
        </button>
      </div>
      {link.crawlInstructions && (
        <div className="mt-1.5">
          <div className="flex flex-wrap gap-2">
            {run?.status === 'running' ? (
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
              className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
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
      {link.jobDetailCrawlInstructions && (
        <div className="mt-1.5">
          <div className="flex flex-wrap gap-2">
            {run?.status === 'running' ? (
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
                  title="Stop watching this crawl in this tab — it keeps running on the server regardless, and reopening this page later will show its latest status."
                  className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                >
                  <PlayIcon />
                  Stop watching
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={handleCrawlJobDetailsNow}
                disabled={isBusy()}
                title="Deterministic — no AI, no platform needed. Visits every already-saved job's own detail page for this link (up to 200 at a time, oldest-crawled first) and updates its description using this link's own job detail crawl instructions. Runs on the server, so it keeps going even if you leave this page."
                className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <PlayIcon />
                Crawl job details now
              </button>
            )}
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
            (run.phase === 'awaiting_human_challenge' ? (
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
        title={activeInstructionsModal === 'jobDetail' ? 'Job detail crawl instructions' : 'Crawl instructions'}
        onClose={() => {
          setActiveInstructionsModal(null)
        }}
      >
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
      </Modal>
    </div>
  )
}
