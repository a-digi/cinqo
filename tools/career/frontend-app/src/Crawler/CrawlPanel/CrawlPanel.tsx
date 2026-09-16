import { useEffect, useRef, useState } from 'react'
import { updatePortalLink, type PortalLink } from '../../api'
import type { Platform } from '../../Cinqo/Platform/platformRepository'
import { CoreApiError } from '../../Cinqo/Http/client'
import { createConversation, sendMessage } from '../../Cinqo/Conversation/conversation'
import { buildCrawlMessage, crawlConversationTitle } from '../crawl'
import { buildGenerateInstructionsMessage, generateInstructionsConversationTitle } from '../generateInstructions'
import { startCrawlNow, fetchActiveCrawlRun, type CrawlRun } from '../crawlNow'
import { PlayIcon, SparkleIcon, LogIcon, AlertIcon } from '../../Shared/Icons/icons'

// PHASE_LABELS (step 40) — a human-readable sentence per fine-grained
// crawl_runs.phase value (step 39). A plain lookup, not a switch,
// since crawlNow.ts's own CrawlRun.phase is deliberately a plain
// string, not a TS union — see that field's own doc comment.
const PHASE_LABELS: Record<string, string> = {
  building_request: 'Preparing crawl instructions',
  navigating: 'Opening the page',
  checking_cloudflare: 'Checking for a Cloudflare challenge',
  awaiting_human_challenge: 'Waiting for you to solve a Cloudflare challenge',
  extracting: 'Extracting job listings',
  ingesting_jobs: 'Saving jobs',
}

// phaseLabel falls back to the raw phase string for a value not yet in
// PHASE_LABELS (e.g. a phase the backend adds later than this table) —
// still shows *something* meaningful rather than nothing.
function phaseLabel(phase: string | null): string | null {
  if (!phase) return null
  return PHASE_LABELS[phase] ?? phase
}

// A crawl run's own log is sparse (four or five entries total), so a
// tighter poll than this (coarser than the AI conversation feature's
// own 2s turn poll) would buy nothing.
const CRAWL_POLL_INTERVAL_MS = 3000

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// latestCrawlLogMessage strips the leading "RFC3339<TAB>" the
// backend's own log entries carry (crawl_runs.go's appendCrawlRunLog)
// and returns just the human-readable message part of the most recent
// one — the live "what's happening right now" text shown next to the
// "Stop watching" button while a run is still in progress.
function latestCrawlLogMessage(log: string[]): string | null {
  if (log.length === 0) return null
  const last = log[log.length - 1]
  const tabIndex = last.indexOf('\t')
  return tabIndex >= 0 ? last.slice(tabIndex + 1) : last
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

  const [instructionsExpanded, setInstructionsExpanded] = useState(false)
  const [instructionsDraft, setInstructionsDraft] = useState('')

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
  // cancel anything server-side (Career's own goroutine has no cancel
  // path at all, per step-37's own design: a crawl run has no natural
  // mid-flight abort point the way an LLM tool-calling loop does).
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

  // "Generate with AI" (step 33) — creates a conversation the same way
  // handleCrawlWithAI does, but hidden (never shown in the normal
  // conversation list) and instructed to write/update this link's own
  // crawl_instructions instead of running a crawl. Runs in the
  // background regardless of this tab staying open
  // (plan/ai/conversation/step-23-detach-turn-execution-from-request.md);
  // only a failure is durably recorded (updatePortalLink's own
  // instructionsAiError field) — success clears any previous failure
  // and needs no further trace.
  function handleGenerateInstructionsWithAI() {
    if (aiPending || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setAiPending(true)
    setAiLocalError(undefined)

    void createConversation({ title: generateInstructionsConversationTitle(link), platformId, model, hidden: true })
      .then((conversation) => sendMessage(conversation.id, buildGenerateInstructionsMessage(link)))
      .then(() => updatePortalLink(link.id, { instructionsAiError: '' }))
      .catch((err: unknown) => {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'Failed to generate crawl instructions.'
        // Shown immediately, regardless of whether the durable write
        // below succeeds — a network failure recording the error must
        // never leave the failure completely invisible.
        setAiLocalError(text)
        return updatePortalLink(link.id, { instructionsAiError: text }).catch((recordErr: unknown) => {
          // Recording the failure durably itself also failed — the
          // local error above still shows for this session; see
          // step 33's own Open Question 1 for the reliability limit
          // durable recording already has (e.g. across a reload).
          console.error('failed to record instructions-AI error on portal link', link.id, recordErr)
        })
      })
      .then(() => {
        onReload()
      })
      .finally(() => {
        setAiPending(false)
      })
  }

  function handleViewConversation() {
    window.__cinqoToolBridge.navigate('/conversations')
  }

  function toggleCrawlInstructions() {
    if (instructionsExpanded) {
      setInstructionsExpanded(false)
      return
    }
    setInstructionsExpanded(true)
    setInstructionsDraft(link.crawlInstructions ?? '')
  }

  function handleSaveCrawlInstructions() {
    updatePortalLink(link.id, { crawlInstructions: instructionsDraft })
      .then(() => {
        setInstructionsExpanded(false)
        onReload()
      })
      .catch((err: unknown) => {
        onError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="mt-1.5">
      <button type="button" onClick={toggleCrawlInstructions} className="text-xs text-gray-500 underline hover:text-gray-700">
        {link.crawlInstructions ? 'Crawl instructions set' : 'No crawl instructions yet'} —{' '}
        {instructionsExpanded ? 'hide' : link.crawlInstructions ? 'view/edit' : 'add'}
      </button>
      {' · '}
      <button
        type="button"
        onClick={handleGenerateInstructionsWithAI}
        disabled={aiPending || !hasPlatforms}
        title={
          !hasPlatforms
            ? 'No AI platform configured — add one on the Platforms page first'
            : 'Let the AI inspect this page and write (or update) its crawl instructions for you'
        }
        className="inline-flex items-center gap-1 text-xs text-gray-500 underline hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <SparkleIcon />
        {aiPending ? 'Generating…' : 'Generate with AI'}
      </button>
      {(aiLocalError ?? link.instructionsAiError) && (
        <p className="mt-1 text-xs text-red-700">
          AI instructions generation failed: {aiLocalError ?? link.instructionsAiError}
          {!aiLocalError && link.instructionsAiErrorAt && ` (${new Date(link.instructionsAiErrorAt).toLocaleString()})`}
        </p>
      )}
      {link.crawlInstructions && (
        <div className="mt-1.5">
          <div className="flex flex-wrap gap-2">
            {run?.status === 'running' ? (
              <button
                type="button"
                onClick={handleCancelCrawlNow}
                title="Stop watching this crawl in this tab — it keeps running on the server regardless, and reopening this page later will show its latest status. See plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md."
                className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
              >
                <PlayIcon />
                Stop watching
              </button>
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
          {/* "Crawl now"'s own live/terminal state (steps 37/38/40) —
              kept separate from crawlResult above, which only ever
              carries a start failure or a stop notice for this
              mechanism. Shown for every status, including while
              running: the backend now tracks a real, fine-grained
              phase per step (step 39, sourced from browser's own live
              status — step 31) and this instance polls it every 3s —
              surfacing the current phase live, plus the full log on
              demand, is what "the frontend shows the status and the
              logs" concretely means (not just on a terminal outcome).
              awaiting_human_challenge gets its own visually distinct
              callout — this is the one phase where the user, not the
              system, is the blocker. */}
          {run && (
            <div className="mt-1">
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
        </div>
      )}
      {instructionsExpanded && (
        <div className="mt-1.5">
          <textarea
            value={instructionsDraft}
            onChange={(e) => {
              setInstructionsDraft(e.target.value)
            }}
            placeholder={'fields:\n  - label: title\n    selector: h1\npagination:\n  nextSelector: a.next-page\n  maxPages: 5'}
            rows={6}
            spellCheck={false}
            className="w-full rounded-md border border-gray-300 px-2 py-1.5 font-mono text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <div className="mt-1.5 flex gap-2">
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
                setInstructionsExpanded(false)
              }}
              className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
