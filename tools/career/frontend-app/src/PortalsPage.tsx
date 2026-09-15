import { useEffect, useRef, useState } from 'react'
import {
  fetchPortals,
  createPortal,
  updatePortal,
  removePortal,
  addPortalLink,
  updatePortalLink,
  removePortalLink,
  type Portal,
  type PortalLink,
} from './api'
import { fetchPlatforms, createConversation, sendMessage, CoreApiError, type Platform } from './coreApi'
import { buildCrawlMessage, crawlConversationTitle } from './crawl'
import { buildGenerateInstructionsMessage, generateInstructionsConversationTitle } from './generateInstructions'
import { startCrawlNow, fetchActiveCrawlRun, type CrawlRun } from './crawlNow'
import { Dropdown } from './Dropdown'
import { PlusIcon, PlayIcon, SparkleIcon } from './icons'

// Portals are tool-wide, not persona/profile-scoped — same reasoning
// Jobs/Companies already document. No Dropdown call site here: a
// portal's own links are a free-form add/remove list, not a
// single-select picker. Each link's own crawl instructions (step 19)
// are shown/edited as raw YAML text — no client-side parsing or
// validation, the backend's own shape check is the only one. See
// plan/ai/tools/career/step-21-portals-frontend.md.
export function PortalsPage() {
  const [portals, setPortals] = useState<Portal[]>([])
  const [newPortalName, setNewPortalName] = useState('')
  const [editingPortalId, setEditingPortalId] = useState<string | null>(null)
  const [editPortalName, setEditPortalName] = useState('')
  const [linkUrlDrafts, setLinkUrlDrafts] = useState<Record<string, string>>({})
  const [linkTitleDrafts, setLinkTitleDrafts] = useState<Record<string, string>>({})
  const [editingLinkId, setEditingLinkId] = useState<string | null>(null)
  const [editLinkUrl, setEditLinkUrl] = useState('')
  const [editLinkTitle, setEditLinkTitle] = useState('')
  const [expandedLinkId, setExpandedLinkId] = useState<string | null>(null)
  const [crawlInstructionsDraft, setCrawlInstructionsDraft] = useState('')
  const [error, setError] = useState('')

  // Manual crawl trigger — two independent mechanisms:
  //   - "Crawl now" (steps 27/37/38): deterministic, no AI. As of step
  //     37, the whole navigate/extract/ingest sequence runs detached on
  //     Career's own backend (crawl_now.go) — this page only starts it
  //     (startCrawlNow) and polls its live state (fetchActiveCrawlRun),
  //     tracked in crawlRunWatches below, not crawlingLinkIds/
  //     crawlResults. Surviving a closed tab / page reload is the
  //     whole point; see the resume-on-load effect further down.
  //   - "Crawl with AI" (steps 24-26, kept per step 27's own decision
  //     to offer both rather than replace): creates a conversation and
  //     sends a generated instruction, letting the AI itself drive
  //     browser's tools — needs a configured platform/API key, but
  //     tolerates crawl instructions that don't fit "Crawl now"'s own
  //     fixed label vocabulary and can apply its own judgement
  //     (normalizing dates, inferring a missing company name, etc).
  //     Still uses crawlingLinkIds/crawlResults exactly as before.
  // platforms/selectedPlatformId/selectedModel only matter for "Crawl
  // with AI" — "Crawl now" needs none of them.
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [selectedPlatformId, setSelectedPlatformId] = useState<string | null>(null)
  const [selectedModel, setSelectedModel] = useState<string | null>(null)
  const [crawlingLinkIds, setCrawlingLinkIds] = useState<Set<string>>(new Set())
  const [crawlResults, setCrawlResults] = useState<Record<string, { ok: boolean; text: string; conversationId?: string } | undefined>>({})

  // isLinkBusy is the one place "is anything already crawling this
  // link" is decided — true while either mechanism is active, so
  // starting one disables the other's own button, matching the
  // cross-exclusion the old shared crawlingLinkIds Set gave both
  // mechanisms before step 37 split "Crawl now"'s own busy state out
  // into crawlRunWatches.
  function isLinkBusy(id: string): boolean {
    return crawlingLinkIds.has(id) || crawlRunWatches[id]?.status === 'running'
  }

  // "Generate with AI" (step 33) — a separate, independent busy-state
  // from crawlingLinkIds above: generating/editing a link's own
  // crawl_instructions is a different operation from crawling it, and
  // could in principle overlap. No local result state on success —
  // success needs no visible trace beyond the spinner clearing (and,
  // if instructions were written, the existing "Crawl instructions
  // set" toggle text updating on the next load()).
  //
  // instructionsAiLocalError (below) is a same-session, immediate
  // failure display — set the moment the AI attempt itself fails,
  // never waiting on (or depending on) the durable
  // updatePortalLink(instructionsAiError) write that follows it also
  // succeeding. Without this, a failure of that *recording* call
  // itself (a separate network request, which can fail for its own
  // reasons) would leave nothing visible at all — exactly the bug
  // this fixes. link.instructionsAiError (the durable, reload-
  // surviving field from the portals payload) is still the source of
  // truth once load() refreshes; this local copy is only a same-
  // session guarantee that a failure is never silently invisible. See
  // plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
  const [instructionsAiPendingIds, setInstructionsAiPendingIds] = useState<Set<string>>(new Set())
  const [instructionsAiLocalError, setInstructionsAiLocalError] = useState<Record<string, string | undefined>>({})

  function load() {
    setError('')
    fetchPortals()
      .then(setPortals)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
    fetchPlatforms()
      .then((list) => {
        setPlatforms(list)
        if (list.length > 0) {
          setSelectedPlatformId(list[0].id)
          setSelectedModel(list[0].models.length > 0 ? list[0].models[0] : null)
        }
      })
      .catch(() => {
        // Left empty (no platforms) rather than surfacing this as a
        // page-level error — every "Crawl now" button already
        // disables itself when platforms.length === 0, which is
        // enough signal on its own.
      })
  }, [])

  // Resume watching any "Crawl now" run still in progress — the
  // concrete mechanism behind "the user can leave the page, it does
  // not have to be in the tab": reopening/reloading this page re-runs
  // load(), which brings back each link's own hasActiveCrawlRun, and
  // this effect immediately starts polling any that are still running
  // without requiring a click.
  //
  // Guarded by `!(link.id in crawlWatchTokensRef.current)`, not by
  // crawlRunWatches' own status — crawlWatchTokensRef gets an entry
  // for a link the FIRST time this tab ever calls watchCrawlRun for
  // it (from this effect or from handleCrawlNow) and that entry is
  // never removed, including by handleCancelCrawlNow (which only
  // bumps it to supersede the poll, on purpose). Using the run's own
  // status instead would reopen a watch this tab's user just
  // explicitly stopped the moment any unrelated load() refresh ran
  // (e.g. editing a different link) — a real bug caught while writing
  // this effect, not a hypothetical: hasActiveCrawlRun stays true on
  // the server regardless of what one tab's own UI decided to stop
  // showing. This condition instead means "resume once per link, per
  // page load, only for a link this tab hasn't already made its own
  // explicit decision about" (steps 37/38).
  useEffect(() => {
    for (const portal of portals) {
      for (const link of portal.links) {
        if (link.hasActiveCrawlRun && !(link.id in crawlWatchTokensRef.current)) {
          void watchCrawlRun(link.id)
        }
      }
    }
  }, [portals])

  const selectedPlatform = platforms.find((p) => p.id === selectedPlatformId) ?? null

  function handleSelectPlatform(id: string) {
    setSelectedPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setSelectedModel(next && next.models.length > 0 ? next.models[0] : null)
  }

  function startCrawl(link: PortalLink) {
    setCrawlingLinkIds((prev) => new Set(prev).add(link.id))
    setCrawlResults((prev) => ({ ...prev, [link.id]: undefined }))
  }

  function finishCrawl(link: PortalLink) {
    setCrawlingLinkIds((prev) => {
      const next = new Set(prev)
      next.delete(link.id)
      return next
    })
  }

  // "Crawl now" own live state, keyed by portal link id (steps 37/38)
  // — the authoritative record of what Career's own detached backend
  // goroutine has reported for this link's most recent run, whether
  // still running or already terminal. Independent of crawlingLinkIds/
  // crawlResults above (those now belong to "Crawl with AI" only).
  const [crawlRunWatches, setCrawlRunWatches] = useState<Record<string, CrawlRun>>({})
  // Per-link cancellation token (same pattern step 35's own
  // crawlNowTokensRef used, and ConversationContext.tsx's own
  // watchTokensRef) — lets "Stop watching" supersede an in-flight poll
  // loop cleanly without an AbortController.
  const crawlWatchTokensRef = useRef<Record<string, number>>({})
  // Which link's own failure log is currently expanded — at most one
  // at a time, matching expandedLinkId's own single-open convention
  // elsewhere on this page.
  const [expandedCrawlLogLinkId, setExpandedCrawlLogLinkId] = useState<string | null>(null)

  function sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms))
  }

  // watchCrawlRun polls GET .../crawl-now/active until the run reaches
  // a terminal status, writing every observed row into crawlRunWatches
  // as it goes — this is what "the frontend shows the status and the
  // logs" means concretely: crawlRunWatches[link.id] is always exactly
  // what the backend's own crawl_runs row says, never optimistic local
  // state. A poll interval of 3s (coarser than the AI conversation
  // feature's own 2s turn poll) — a crawl run's own log is sparse
  // (four or five entries total), so tighter polling buys nothing.
  const CRAWL_POLL_INTERVAL_MS = 3000
  async function watchCrawlRun(portalLinkId: string) {
    const myToken = (crawlWatchTokensRef.current[portalLinkId] ?? 0) + 1
    crawlWatchTokensRef.current[portalLinkId] = myToken

    for (;;) {
      let run: CrawlRun | null
      try {
        run = await fetchActiveCrawlRun(portalLinkId)
      } catch {
        break
      }
      if (crawlWatchTokensRef.current[portalLinkId] !== myToken) return // superseded
      if (!run) break
      setCrawlRunWatches((prev) => ({ ...prev, [portalLinkId]: run! }))
      if (run.status !== 'running') break
      await sleep(CRAWL_POLL_INTERVAL_MS)
      if (crawlWatchTokensRef.current[portalLinkId] !== myToken) return
    }
  }

  // "Crawl now" (steps 27/37/38) — starts the crawl on Career's own
  // backend and begins watching it; the actual navigate/extract/ingest
  // sequence runs entirely server-side from this point on; closing
  // this tab does not stop it (see step-37's own design). A start
  // failure (e.g. 409 — one already running, or 400 — no valid crawl
  // instructions) is shown via the existing crawlResults notice, the
  // same place "Crawl with AI" shows its own outcomes; the run's own
  // later live/terminal state always lives in crawlRunWatches instead.
  function handleCrawlNow(link: PortalLink) {
    if (isLinkBusy(link.id)) return
    setCrawlResults((prev) => ({ ...prev, [link.id]: undefined }))
    startCrawlNow(link.id)
      .then((started) => {
        setCrawlRunWatches((prev) => ({
          ...prev,
          [link.id]: {
            crawlRunId: started.crawlRunId,
            status: 'running',
            startedAt: started.startedAt,
            finishedAt: null,
            log: [],
            resultSummary: null,
            errorMessage: null,
          },
        }))
        return watchCrawlRun(link.id)
      })
      .catch((err: unknown) => {
        setCrawlResults((prev) => ({ ...prev, [link.id]: { ok: false, text: err instanceof Error ? err.message : 'Failed to start crawl.' } }))
      })
  }

  // "Stop watching" only ever bumps the token so this tab stops
  // polling and clears its own local view of the run — it does NOT
  // cancel anything server-side (Career's own goroutine has no cancel
  // path at all, per step-37's own design: a crawl run has no natural
  // mid-flight abort point the way an LLM tool-calling loop does).
  // Reopening this page later re-discovers the run via
  // hasActiveCrawlRun/the resume effect below if it's still going, or
  // shows its finished result if it already completed.
  function handleCancelCrawlNow(link: PortalLink) {
    crawlWatchTokensRef.current[link.id] = (crawlWatchTokensRef.current[link.id] ?? 0) + 1
    setCrawlRunWatches((prev) => {
      const next = { ...prev }
      delete next[link.id]
      return next
    })
    setCrawlResults((prev) => ({
      ...prev,
      [link.id]: { ok: false, text: 'Stopped watching in this tab — the crawl may still be running in the background; reopen this page to check its latest status.' },
    }))
  }

  // "Crawl with AI" (steps 24-26, kept per step 27) — creates a
  // conversation and lets the AI itself drive browser's tools, using
  // its own judgement rather than the fixed label vocabulary above.
  function handleCrawlWithAI(link: PortalLink) {
    if (isLinkBusy(link.id) || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    startCrawl(link)

    createConversation({ title: crawlConversationTitle(link), platformId, model })
      .then((conversation) => sendMessage(conversation.id, buildCrawlMessage(link)).then((result) => ({ conversation, result })))
      .then(({ conversation, result }) => {
        const suffix = result.durationMs != null ? ` (${(result.durationMs / 1000).toFixed(1)}s)` : ''
        setCrawlResults((prev) => ({
          ...prev,
          [link.id]: { ok: true, text: `${result.content}${suffix}`, conversationId: conversation.id },
        }))
      })
      .catch((err: unknown) => {
        const text =
          err instanceof CoreApiError && (err.status === 401 || err.status === 403)
            ? 'Ask an admin to grant you access to AI conversations.'
            : err instanceof Error
              ? err.message
              : 'Crawl failed.'
        setCrawlResults((prev) => ({ ...prev, [link.id]: { ok: false, text } }))
      })
      .finally(() => finishCrawl(link))
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
  function handleGenerateInstructionsWithAI(link: PortalLink) {
    if (instructionsAiPendingIds.has(link.id) || !selectedPlatform) return
    const platformId = selectedPlatform.id
    const model = selectedPlatform.models.length > 0 ? (selectedModel ?? selectedPlatform.models[0]) : undefined
    setInstructionsAiPendingIds((prev) => new Set(prev).add(link.id))
    setInstructionsAiLocalError((prev) => ({ ...prev, [link.id]: undefined }))

    createConversation({ title: generateInstructionsConversationTitle(link), platformId, model, hidden: true })
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
        // never leave the failure completely invisible. See this
        // handler's own top comment.
        setInstructionsAiLocalError((prev) => ({ ...prev, [link.id]: text }))
        return updatePortalLink(link.id, { instructionsAiError: text }).catch((recordErr: unknown) => {
          // Recording the failure durably itself also failed — the
          // local error above still shows for this session; see this
          // step's own Open Question 1 for the reliability limit
          // durable recording already has (e.g. across a reload).
          console.error('failed to record instructions-AI error on portal link', link.id, recordErr)
        })
      })
      .then(() => load())
      .finally(() => {
        setInstructionsAiPendingIds((prev) => {
          const next = new Set(prev)
          next.delete(link.id)
          return next
        })
      })
  }

  function handleViewConversation() {
    window.__cinqoToolBridge.navigate('/conversations')
  }

  function handleCreatePortal() {
    const name = newPortalName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    createPortal(name)
      .then(() => {
        setNewPortalName('')
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  function startEditPortal(p: Portal) {
    setEditingPortalId(p.id)
    setEditPortalName(p.name)
  }

  function cancelEditPortal() {
    setEditingPortalId(null)
    setEditPortalName('')
  }

  function saveEditPortal(id: string) {
    const name = editPortalName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    updatePortal(id, name)
      .then(() => {
        cancelEditPortal()
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleDeletePortal(id: string) {
    setError('')
    removePortal(id)
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  function linkUrlDraft(portalId: string): string {
    return linkUrlDrafts[portalId] ?? ''
  }

  function linkTitleDraft(portalId: string): string {
    return linkTitleDrafts[portalId] ?? ''
  }

  function handleAddLink(portalId: string) {
    const url = linkUrlDraft(portalId).trim()
    const title = linkTitleDraft(portalId).trim()
    if (!url) {
      setError('URL is required')
      return
    }
    if (!title) {
      setError('Title is required')
      return
    }
    setError('')
    addPortalLink(portalId, url, title)
      .then(() => {
        setLinkUrlDrafts((prev) => ({ ...prev, [portalId]: '' }))
        setLinkTitleDrafts((prev) => ({ ...prev, [portalId]: '' }))
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  function startEditLink(link: PortalLink) {
    setEditingLinkId(link.id)
    setEditLinkUrl(link.url)
    setEditLinkTitle(link.title ?? '')
  }

  function cancelEditLink() {
    setEditingLinkId(null)
    setEditLinkUrl('')
    setEditLinkTitle('')
  }

  function saveEditLink(id: string) {
    const url = editLinkUrl.trim()
    const title = editLinkTitle.trim()
    if (!url) {
      setError('URL is required')
      return
    }
    if (!title) {
      setError('Title is required')
      return
    }
    setError('')
    updatePortalLink(id, { url, title })
      .then(() => {
        cancelEditLink()
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleRemoveLink(id: string) {
    setError('')
    removePortalLink(id)
      .then(load)
      .catch((err: Error) => setError(err.message))
  }

  function toggleCrawlInstructions(link: PortalLink) {
    if (expandedLinkId === link.id) {
      setExpandedLinkId(null)
      return
    }
    setExpandedLinkId(link.id)
    setCrawlInstructionsDraft(link.crawlInstructions ?? '')
  }

  function handleSaveCrawlInstructions(linkId: string) {
    setError('')
    updatePortalLink(linkId, { crawlInstructions: crawlInstructionsDraft })
      .then(() => {
        setExpandedLinkId(null)
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Portals</h1>
      <p className="mb-5 text-sm text-gray-500">
        Job portals to crawl — each portal owns one or more links, and each link can carry its own
        YAML crawl instructions (the exact shape the browser tool's own crawl_paginated expects),
        normally prepared by the AI. Deleting a portal permanently deletes every link it owns.
      </p>

      {platforms.length > 1 && (
        <div className="mb-5 flex flex-wrap items-end gap-3 rounded-md border border-gray-200 bg-gray-50 p-3">
          <div className="min-w-[180px]">
            <label className="mb-1 block text-xs font-medium text-gray-500">Crawl using</label>
            <Dropdown
              options={platforms.map((p) => ({ value: p.id, label: p.name }))}
              value={selectedPlatformId}
              onChange={handleSelectPlatform}
            />
          </div>
          {selectedPlatform && selectedPlatform.models.length > 0 && (
            <div className="min-w-[180px]">
              <label className="mb-1 block text-xs font-medium text-gray-500">Model</label>
              <Dropdown
                options={selectedPlatform.models.map((m) => ({ value: m, label: m }))}
                value={selectedModel}
                onChange={setSelectedModel}
              />
            </div>
          )}
        </div>
      )}

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="mb-4 space-y-4">
        {portals.map((p) => (
          <div key={p.id} className="rounded-md border border-gray-200 p-3">
            {editingPortalId === p.id ? (
              <div className="mb-3">
                <input
                  autoFocus
                  value={editPortalName}
                  onChange={(e) => setEditPortalName(e.target.value)}
                  placeholder="Name"
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => saveEditPortal(p.id)}
                    className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={cancelEditPortal}
                    className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <div className="mb-3 flex items-start justify-between gap-3">
                <div className="text-sm font-medium">{p.name}</div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => startEditPortal(p)}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDeletePortal(p.id)}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
                  >
                    Delete
                  </button>
                </div>
              </div>
            )}

            <div className="space-y-2">
              {p.links.map((link) => (
                <div key={link.id} className="rounded-md border border-gray-100 bg-gray-50 p-2">
                  {editingLinkId === link.id ? (
                    <div className="space-y-1.5">
                      <input
                        autoFocus
                        value={editLinkTitle}
                        onChange={(e) => setEditLinkTitle(e.target.value)}
                        placeholder="Title, e.g. Software Engineer jobs, Hamburg"
                        className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                      />
                      <input
                        value={editLinkUrl}
                        onChange={(e) => setEditLinkUrl(e.target.value)}
                        placeholder="URL"
                        className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => saveEditLink(link.id)}
                          className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          onClick={cancelEditLink}
                          className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <a
                          href={link.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="block truncate text-xs font-medium text-gray-900 underline hover:text-gray-700"
                        >
                          {link.title || '(untitled link)'}
                        </a>
                        <div className="truncate text-xs text-gray-500">{link.url}</div>
                        <div className="truncate text-xs text-gray-400">
                          {link.lastCrawledAt ? `Last crawled: ${new Date(link.lastCrawledAt).toLocaleString()}` : 'Never crawled'}
                        </div>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => startEditLink(link)}
                          className="text-xs text-gray-500 underline hover:text-gray-700"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => handleRemoveLink(link.id)}
                          className="text-xs text-red-700 underline hover:text-red-800"
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  )}

                  <div className="mt-1.5">
                    <button
                      type="button"
                      onClick={() => toggleCrawlInstructions(link)}
                      className="text-xs text-gray-500 underline hover:text-gray-700"
                    >
                      {link.crawlInstructions ? 'Crawl instructions set' : 'No crawl instructions yet'} —{' '}
                      {expandedLinkId === link.id ? 'hide' : link.crawlInstructions ? 'view/edit' : 'add'}
                    </button>
                    {' · '}
                    <button
                      type="button"
                      onClick={() => handleGenerateInstructionsWithAI(link)}
                      disabled={instructionsAiPendingIds.has(link.id) || platforms.length === 0}
                      title={
                        platforms.length === 0
                          ? 'No AI platform configured — add one on the Platforms page first'
                          : 'Let the AI inspect this page and write (or update) its crawl instructions for you'
                      }
                      className="inline-flex items-center gap-1 text-xs text-gray-500 underline hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <SparkleIcon />
                      {instructionsAiPendingIds.has(link.id) ? 'Generating…' : 'Generate with AI'}
                    </button>
                    {(instructionsAiLocalError[link.id] || link.instructionsAiError) && (
                      <p className="mt-1 text-xs text-red-700">
                        AI instructions generation failed: {instructionsAiLocalError[link.id] || link.instructionsAiError}
                        {!instructionsAiLocalError[link.id] && link.instructionsAiErrorAt && ` (${new Date(link.instructionsAiErrorAt).toLocaleString()})`}
                      </p>
                    )}
                    {link.crawlInstructions && (
                      <div className="mt-1.5">
                        <div className="flex flex-wrap gap-2">
                          {crawlRunWatches[link.id]?.status === 'running' ? (
                            <button
                              type="button"
                              onClick={() => handleCancelCrawlNow(link)}
                              title="Stop watching this crawl in this tab — it keeps running on the server regardless, and reopening this page later will show its latest status. See plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md."
                              className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                            >
                              <PlayIcon />
                              Stop watching
                            </button>
                          ) : (
                            <button
                              type="button"
                              onClick={() => handleCrawlNow(link)}
                              disabled={isLinkBusy(link.id)}
                              title="Deterministic — no AI, no platform needed. Runs on the server, so it keeps going even if you leave this page. Requires fields labeled title/url (and optionally company/location/description/postedAt) in this link's crawl instructions. If Cloudflare blocks it, this can take up to about 30 minutes while the browser tool waits for it to clear."
                              className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
                            >
                              <PlayIcon />
                              Crawl now
                            </button>
                          )}
                          <button
                            type="button"
                            onClick={() => handleCrawlWithAI(link)}
                            disabled={isLinkBusy(link.id) || platforms.length === 0}
                            title={
                              platforms.length === 0
                                ? 'No AI platform configured — add one on the Platforms page first'
                                : 'Lets the AI drive the crawl itself — tolerates instructions that don’t fit "Crawl now"’s fixed fields, but needs a configured AI platform.'
                            }
                            className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
                          >
                            <PlayIcon />
                            {crawlingLinkIds.has(link.id) ? 'Crawling…' : 'Crawl with AI'}
                          </button>
                        </div>
                        {crawlResults[link.id] && (
                          <p className={`mt-1 text-xs ${crawlResults[link.id]!.ok ? 'text-green-700' : 'text-red-700'}`}>
                            {crawlResults[link.id]!.text}
                            {crawlResults[link.id]!.ok && crawlResults[link.id]!.conversationId && (
                              <>
                                {' — '}
                                <button
                                  type="button"
                                  onClick={handleViewConversation}
                                  className="underline hover:text-green-900"
                                >
                                  View conversation
                                </button>
                              </>
                            )}
                          </p>
                        )}
                        {/* "Crawl now"'s own live/terminal state (steps 37/38) — kept
                            separate from crawlResults above, which now only ever
                            carries a start failure or a stop notice for this
                            mechanism. Nothing shown while running: the button itself
                            already says "Stop watching". */}
                        {crawlRunWatches[link.id]?.status === 'completed' && (
                          <p className="mt-1 text-xs text-green-700">{crawlRunWatches[link.id]!.resultSummary}</p>
                        )}
                        {crawlRunWatches[link.id]?.status === 'failed' && (
                          <div className="mt-1">
                            <p className="text-xs text-red-700">{crawlRunWatches[link.id]!.errorMessage}</p>
                            <button
                              type="button"
                              onClick={() => setExpandedCrawlLogLinkId(expandedCrawlLogLinkId === link.id ? null : link.id)}
                              className="text-xs text-gray-500 underline hover:text-gray-700"
                            >
                              {expandedCrawlLogLinkId === link.id ? 'Hide log' : 'View log'}
                            </button>
                            {expandedCrawlLogLinkId === link.id && (
                              <pre className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap rounded-md bg-gray-900 p-2 text-xs text-gray-100">
                                {crawlRunWatches[link.id]!.log.length > 0 ? crawlRunWatches[link.id]!.log.join('\n') : '(no log entries)'}
                              </pre>
                            )}
                          </div>
                        )}
                      </div>
                    )}
                    {expandedLinkId === link.id && (
                      <div className="mt-1.5">
                        <textarea
                          value={crawlInstructionsDraft}
                          onChange={(e) => setCrawlInstructionsDraft(e.target.value)}
                          placeholder={'fields:\n  - label: title\n    selector: h1\npagination:\n  nextSelector: a.next-page\n  maxPages: 5'}
                          rows={6}
                          spellCheck={false}
                          className="w-full rounded-md border border-gray-300 px-2 py-1.5 font-mono text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                        />
                        <div className="mt-1.5 flex gap-2">
                          <button
                            type="button"
                            onClick={() => handleSaveCrawlInstructions(link.id)}
                            className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
                          >
                            Save
                          </button>
                          <button
                            type="button"
                            onClick={() => setExpandedLinkId(null)}
                            className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-white"
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-2 flex flex-wrap gap-2">
              <input
                value={linkTitleDraft(p.id)}
                onChange={(e) => setLinkTitleDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))}
                placeholder="Title, e.g. Software Engineer jobs, Hamburg"
                className="min-w-[220px] flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
              />
              <input
                value={linkUrlDraft(p.id)}
                onChange={(e) => setLinkUrlDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))}
                placeholder="URL to crawl"
                className="min-w-[220px] flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
              />
              <button
                type="button"
                onClick={() => handleAddLink(p.id)}
                className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
              >
                <PlusIcon />
                Add link
              </button>
            </div>
          </div>
        ))}
      </div>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">New portal</h2>
        <input
          value={newPortalName}
          onChange={(e) => setNewPortalName(e.target.value)}
          placeholder="Name"
          className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={handleCreatePortal}
          className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          Create portal
        </button>
      </section>
    </div>
  )
}
