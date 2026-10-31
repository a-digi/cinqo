import { useEffect, useState } from 'react'
import {
  fetchPortals,
  createPortal,
  updatePortal,
  removePortal,
  addPortalLink,
  updatePortalLink,
  removePortalLink,
  setPortalAi,
  setPortalLinkAi,
  inheritPortalInstructions,
  type Portal,
  type PortalLink,
} from '../../api'
import { resolveCrawlAi } from '../../Cinqo/Platform/crawlAiChoice'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { GlobeIcon } from '../../Shared/Icons/icons'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { plural, summarizePortals } from './portalStats'
import { AutoDiscoveryPanel } from '../../Crawler/AutoDiscoveryPanel/AutoDiscoveryPanel'
import { anyLinkAutoDiscoverable } from '../../Crawler/autoDiscoveryVisibility'
import { CrawlPanel } from '../../Crawler/CrawlPanel/CrawlPanel'
import { inheritErrorView, inheritView, type InheritView } from '../../Crawler/inheritOutcome'
import { PlusIcon } from '../../Shared/Icons/icons'
import { Modal } from '../../Shared/Modal/Modal'
import { AccordionItem } from '../../Shared/Accordion/AccordionItem'
import { CrawlAiModal } from './CrawlAiModal'
import { aiChip, describeChoice, linkChip, overrideOf } from './aiOverrideDisplay'
import { PortalLinkHeader } from './PortalLinkHeader'
import { VIEW_PORTAL_PATH } from '../../paths'

// Portals are tool-wide, not persona/profile-scoped — same reasoning
// Jobs/Companies already document. No Dropdown call site here: a
// portal's own links are a free-form add/remove list, not a
// single-select picker. Each link's own crawl instructions (step 19)
// and every crawl-triggering mechanism live in CrawlPanel, one
// instance per link (step 42) — this page owns only portal/link CRUD.
// Portals render as a 2-column (1 on mobile) grid of accordions, and
// "New portal" opens in a modal instead of a bottom-of-page form —
// see plan/ai/tools/career/step-21-portals-frontend.md,
// plan/ai/tools/career/step-42-crawler-folder-reorganization.md,
// plan/ai/tools/career/step-58-portals-page-modal-accordion-redesign.md,
// and plan/ai/tools/career/step-59-portals-add-link-container-overlay.md
// ("Add link" as an in-card overlay instead of an always-visible row).
export function PortalsPage() {
  const [portals, setPortals] = useState<Portal[]>([])
  const [isNewPortalModalOpen, setIsNewPortalModalOpen] = useState(false)
  const [newPortalName, setNewPortalName] = useState('')
  const [openPortalIds, setOpenPortalIds] = useState<Set<string>>(new Set())
  const [editingPortalId, setEditingPortalId] = useState<string | null>(null)
  const [editPortalName, setEditPortalName] = useState('')
  // Only one portal's own "Add link" overlay is open at a time —
  // opening a second closes the first (see the design doc's flagged
  // default).
  const [addingLinkPortalId, setAddingLinkPortalId] = useState<string | null>(null)
  const [linkUrlDrafts, setLinkUrlDrafts] = useState<Record<string, string>>({})
  const [linkTitleDrafts, setLinkTitleDrafts] = useState<Record<string, string>>({})
  const [editingLinkId, setEditingLinkId] = useState<string | null>(null)
  const [editLinkUrl, setEditLinkUrl] = useState('')
  const [editLinkTitle, setEditLinkTitle] = useState('')
  const [error, setError] = useState('')
  // The portal whose AI choice the modal is editing (step 102), or null when it is closed.
  const [aiPortalId, setAiPortalId] = useState<string | null>(null)
  // The link whose AI choice the modal is editing, or null when it is closed.
  const [aiLinkId, setAiLinkId] = useState<string | null>(null)
  // Links a trial of the portal's other instructions is running for (step 103), and what each
  // trial last said. Both are per link and last only as long as this page is open.
  const [inheriting, setInheriting] = useState<Set<string>>(new Set())
  const [inheritNotes, setInheritNotes] = useState<Record<string, InheritView>>({})

  // The AI platform and model CrawlPanel's "Crawl with AI" / "Generate with AI" use
  // are the one saved setting for the whole tool (Career ▸ Settings), not a choice
  // made on this page.
  const ai = useAiChoice()

  // The AI choice in effect for a link (its own, else its portal's, else the tool-wide one);
  // withoutLink leaves the link's own out, which is what removing it would fall back to.
  function crawlAiFor(portal: Portal, link: PortalLink | null, withoutLink = false) {
    return resolveCrawlAi({
      link: link && !withoutLink ? overrideOf(link) : null,
      portal: overrideOf(portal),
      platforms: ai.platforms,
      keyedPlatformIds: ai.keyedPlatformIds,
      saved: ai.saved,
    })
  }

  function load() {
    setError('')
    fetchPortals()
      .then(setPortals)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
  }, [])

  // Escape closes the "Add link" overlay — mirrors Modal's own
  // convention (step 58). Only armed while the overlay is actually
  // open.
  useEffect(() => {
    if (addingLinkPortalId === null) {
      return
    }
    const portalId = addingLinkPortalId
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        cancelAddLink(portalId)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [addingLinkPortalId])

  // Link accordions (step 89) start closed — an empty set, same pattern
  // as openPortalIds. Keyed by link id, so it survives load() reloads.
  const [openLinkIds, setOpenLinkIds] = useState<Set<string>>(new Set())
  function toggleLinkOpen(id: string) {
    setOpenLinkIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  function togglePortalOpen(id: string) {
    setOpenPortalIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
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
        setIsNewPortalModalOpen(false)
        load()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDeletePortal(id: string) {
    setError('')
    removePortal(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function linkUrlDraft(portalId: string): string {
    return linkUrlDrafts[portalId] ?? ''
  }

  function linkTitleDraft(portalId: string): string {
    return linkTitleDrafts[portalId] ?? ''
  }

  // Tries the instructions of the link's portal-mates on it (step 103). Resolves true when the
  // listing instructions were inherited, so the caller can skip the AI run; any failure
  // resolves false (a failed trial must never block the AI) and is shown as a note.
  function tryInherit(linkId: string): Promise<boolean> {
    setInheriting((prev) => new Set(prev).add(linkId))
    setInheritNotes((prev) => Object.fromEntries(Object.entries(prev).filter(([id]) => id !== linkId)))
    return inheritPortalInstructions(linkId)
      .then((res) => {
        setPortals(res.portals)
        const view = inheritView(res)
        if (view.message !== '') setInheritNotes((prev) => ({ ...prev, [linkId]: view }))
        if (view.skipAi) {
          // The saved listing publishes an event that sets the crawl request on the link a moment
          // later; look again once, so the open panel picks the request up and crawls.
          window.setTimeout(load, 2000)
        }
        return view.skipAi
      })
      .catch((err: unknown) => {
        setInheritNotes((prev) => ({ ...prev, [linkId]: inheritErrorView(err) }))
        return false
      })
      .finally(() => {
        setInheriting((prev) => {
          const next = new Set(prev)
          next.delete(linkId)
          return next
        })
      })
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
      .then(({ id }) => {
        setLinkUrlDrafts((prev) => ({ ...prev, [portalId]: '' }))
        setLinkTitleDrafts((prev) => ({ ...prev, [portalId]: '' }))
        setAddingLinkPortalId(null)
        load()
        // Step 103: the first thing for a new link is to try what the portal's other links already
        // use. Silent when the portal has nothing to offer; the result shows in the link's panel.
        void tryInherit(id)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function cancelAddLink(portalId: string) {
    setAddingLinkPortalId(null)
    setError('')
    setLinkUrlDrafts((prev) => ({ ...prev, [portalId]: '' }))
    setLinkTitleDrafts((prev) => ({ ...prev, [portalId]: '' }))
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleRemoveLink(id: string) {
    setError('')
    removePortalLink(id)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const aiPortal = portals.find((p) => p.id === aiPortalId) ?? null
  const aiLinkPortal = portals.find((p) => p.links.some((l) => l.id === aiLinkId)) ?? null
  const aiLink = aiLinkPortal?.links.find((l) => l.id === aiLinkId) ?? null
  const summary = summarizePortals(portals)

  return (
    <div className="career-v-left career-a-glow career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<GlobeIcon />}
        title="Portals"
        description="Job portals to crawl. Each portal has links, and each link can carry its own crawl instructions, normally prepared by the AI."
        pill={{ text: plural(summary.portals, 'portal', 'portals') }}
      />

      {/* Two columns: the portals on the right; on the left the "New portal" button, the numbers
          and the auto-discovery switch (under the list on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <p className="mb-3 text-xs text-gray-500">Deleting a portal permanently deletes every link it owns.</p>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {/* items-start: without it, CSS Grid's default align-items:stretch
          forces every card in a row to match the tallest (open) sibling's
          height, leaving ugly empty space at the bottom of closed cards. */}
          <div className="space-y-4">
            {portals.map((p, index) => (
              <AccordionItem
                key={p.id}
                variant="portal"
                isOpen={openPortalIds.has(p.id)}
                onToggle={() => {
                  togglePortalOpen(p.id)
                }}
                // "both" fill mode, not "backwards" alone: "backwards" only
                // holds the from-keyframe (opacity:0) during the delay
                // *before* the animation starts — once it finishes, fill
                // stops applying entirely and the element falls back to its
                // own static styles. Without a forwards-inclusive fill mode
                // there'd be nothing left setting opacity back to 1, so every
                // card would render, animate in, then vanish the instant the
                // animation ends. "both" = "backwards" (delay) + "forwards"
                // (after it ends) — this was the actual bug, found live via
                // Playwright. See
                // plan/ai/tools/career/step-58-portals-page-modal-accordion-redesign.md.
                className="[animation:fade-in-up_600ms_ease-out_both]"
                header={
                  editingPortalId === p.id ? (
                    // Not a real interaction — only stops the header row's own
                    // onClick (AccordionItem's toggle) from firing while editing.
                    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
                    <div
                      className="flex items-center gap-2"
                      onClick={(e) => {
                        e.stopPropagation()
                      }}
                    >
                      <input
                        // Deliberate: entering edit mode should focus the input
                        // immediately, the same convention most inline-rename UIs use.
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus
                        value={editPortalName}
                        onChange={(e) => {
                          setEditPortalName(e.target.value)
                        }}
                        placeholder="Name"
                        className="min-w-0 flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                      />
                      <button
                        type="button"
                        onClick={() => {
                          saveEditPortal(p.id)
                        }}
                        className="shrink-0 rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                      >
                        Save
                      </button>
                      <button
                        type="button"
                        onClick={cancelEditPortal}
                        className="shrink-0 rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <div className="flex min-w-0 items-center gap-3">
                      <span
                        className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
                        style={{ background: accentFor(p.name).to }}
                        aria-hidden="true"
                      >
                        {initialOf(p.name)}
                      </span>
                      <span className="truncate text-sm font-semibold">{p.name}</span>
                      <span
                        className="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium"
                        style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                      >
                        {plural(p.links.length, 'link', 'links')}
                      </span>
                      {(() => {
                        const chip = aiChip(overrideOf(p), ai.platforms)
                        return (
                          <button
                            type="button"
                            title="Set the AI platform and model for this portal's links"
                            onClick={(e) => {
                              e.stopPropagation()
                              setAiPortalId(p.id)
                            }}
                            className={`shrink-0 rounded-full border px-2.5 py-0.5 text-xs font-medium ${
                              chip.kind === 'default'
                                ? 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50'
                                : chip.kind === 'set'
                                  ? 'border-violet-200 bg-violet-50 text-violet-800 hover:bg-violet-100'
                                  : 'border-amber-300 bg-amber-50 text-amber-800 hover:bg-amber-100'
                            }`}
                          >
                            {chip.text}
                          </button>
                        )
                      })()}
                      <span className="ml-auto flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            window.__cinqoToolBridge.navigate(`${VIEW_PORTAL_PATH}?id=${encodeURIComponent(p.id)}`)
                          }}
                          className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                        >
                          View
                        </button>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            startEditPortal(p)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            handleDeletePortal(p.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </span>
                    </div>
                  )
                }
                // Staggered mount cascade — animation-delay set inline since
                // Tailwind's own utilities have no per-index dynamic value.
                style={{
                  animationDelay: `${index * 90}ms`,
                  borderColor: PAGE_ACCENT.soft,
                  background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)`,
                }}
                overlay={
                  addingLinkPortalId === p.id ? (
                    <>
                      <h3 className="mb-3 text-sm font-semibold text-gray-900">Add link</h3>
                      {/* No flex-1 here — that would stretch to fill the overlay's
                      own min-h (added so the form isn't clipped on a portal
                      with few/no links) and push the buttons down to the
                      bottom instead of right under the URL field. See
                      plan/ai/tools/career/step-59-portals-add-link-container-overlay.md. */}
                      <div className="flex flex-col gap-2">
                        <input
                          // Deliberate: opening the overlay should focus the
                          // first field immediately, same convention every
                          // other inline form on this page already uses.
                          // eslint-disable-next-line jsx-a11y/no-autofocus
                          autoFocus
                          value={linkTitleDraft(p.id)}
                          onChange={(e) => {
                            setLinkTitleDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                          }}
                          placeholder="Title, e.g. Software Engineer jobs, Hamburg"
                          className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                        />
                        <input
                          value={linkUrlDraft(p.id)}
                          onChange={(e) => {
                            setLinkUrlDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                          }}
                          placeholder="URL to crawl"
                          className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                        />
                      </div>
                      <div className="mt-3 flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            handleAddLink(p.id)
                          }}
                          className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
                        >
                          <PlusIcon />
                          Create link
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            cancelAddLink(p.id)
                          }}
                          className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
                        >
                          Cancel
                        </button>
                      </div>
                    </>
                  ) : undefined
                }
              >
                <div className="space-y-3">
                  {/* Each link is a nested accordion, closed by default (step 89):
                  the header carries what you need at a glance and its
                  Edit/Remove, the body is the link's whole CrawlPanel. The
                  body stays mounted while closed, so a closed link still
                  resumes/finishes generations and crawls. */}
                  {p.links.map((link) => {
                    const linkAi = crawlAiFor(p, link)
                    return (
                      <AccordionItem
                        key={link.id}
                        variant="nested"
                        isOpen={openLinkIds.has(link.id)}
                        onToggle={() => {
                          toggleLinkOpen(link.id)
                        }}
                        header={
                          <PortalLinkHeader
                            link={link}
                            isEditing={editingLinkId === link.id}
                            editTitle={editLinkTitle}
                            editUrl={editLinkUrl}
                            onEditTitleChange={setEditLinkTitle}
                            onEditUrlChange={setEditLinkUrl}
                            onStartEdit={() => {
                              startEditLink(link)
                            }}
                            onSaveEdit={() => {
                              saveEditLink(link.id)
                            }}
                            onCancelEdit={cancelEditLink}
                            onRemove={() => {
                              handleRemoveLink(link.id)
                            }}
                            aiChip={linkChip(linkAi)}
                            onEditAi={() => {
                              setAiLinkId(link.id)
                            }}
                          />
                        }
                      >
                        <CrawlPanel
                          link={link}
                          selectedPlatform={linkAi.platform}
                          selectedModel={linkAi.model}
                          hasPlatforms={ai.platforms.length > 0}
                          inheriting={inheriting.has(link.id)}
                          inheritNote={inheritNotes[link.id] ?? null}
                          onTryInherit={() => tryInherit(link.id)}
                          onReload={load}
                          onError={setError}
                        />
                      </AccordionItem>
                    )
                  })}
                </div>

                <div className="mt-2">
                  <button
                    type="button"
                    onClick={() => {
                      setAddingLinkPortalId(p.id)
                    }}
                    className="flex items-center gap-1 rounded-full border border-blue-200 px-3 py-1.5 text-xs text-blue-700 hover:bg-blue-50"
                  >
                    <PlusIcon />
                    Add link
                  </button>
                </div>
              </AccordionItem>
            ))}
          </div>
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-56">
          <button
            type="button"
            onClick={() => {
              setIsNewPortalModalOpen(true)
            }}
            className="mb-4 flex w-full items-center justify-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
          >
            <PlusIcon />
            New portal
          </button>
          <SummaryRail
            title="Overview"
            segments={[
              { value: summary.withInstructions, color: '#16a34a' },
              { value: summary.links - summary.withInstructions, color: '#cbd5e1' },
            ]}
            centerValue={summary.links === 0 ? '–' : `${Math.round((summary.withInstructions / summary.links) * 100)}%`}
            centerLabel="ready to crawl"
            rows={[
              { label: 'Portals', value: String(summary.portals), color: ACCENTS[0].to },
              { label: 'Links', value: String(summary.links), color: ACCENTS[6].to },
              { label: 'With instructions', value: String(summary.withInstructions), color: '#16a34a' },
              { label: 'Never crawled', value: String(summary.neverCrawled), color: ACCENTS[2].to, emphasis: summary.neverCrawled > 0 },
            ]}
          >
            {/* Step 88 — only while some link has crawl instructions: with none,
                there is nothing the global auto-discovery could crawl. */}
            {anyLinkAutoDiscoverable(portals) && <AutoDiscoveryPanel />}
          </SummaryRail>
        </aside>
      </div>

      <Modal
        open={isNewPortalModalOpen}
        title="New portal"
        onClose={() => {
          setIsNewPortalModalOpen(false)
          setNewPortalName('')
        }}
      >
        <input
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          value={newPortalName}
          onChange={(e) => {
            setNewPortalName(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              handleCreatePortal()
            }
          }}
          placeholder="Name"
          className="mb-3 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={handleCreatePortal}
          className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          Create portal
        </button>
      </Modal>

      {aiPortal && (
        <CrawlAiModal
          open
          title={`AI for ${aiPortal.name}`}
          subject="portal"
          current={overrideOf(aiPortal)}
          platforms={ai.platforms}
          fallbackText={describeChoice(crawlAiFor({ ...aiPortal, aiPlatformId: '', aiModel: '' }, null))}
          onClose={() => {
            setAiPortalId(null)
          }}
          onSave={(platformId, model) =>
            setPortalAi(aiPortal.id, platformId, model).then((list) => {
              setPortals(list)
            })
          }
          onRemove={() =>
            setPortalAi(aiPortal.id, '', '').then((list) => {
              setPortals(list)
            })
          }
        />
      )}

      {aiLink && aiLinkPortal && (
        <CrawlAiModal
          open
          title={`AI for ${(aiLink.title ?? '').trim() === '' ? aiLink.url : (aiLink.title ?? '')}`}
          subject="link"
          current={overrideOf(aiLink)}
          platforms={ai.platforms}
          fallbackText={describeChoice(crawlAiFor(aiLinkPortal, aiLink, true))}
          onClose={() => {
            setAiLinkId(null)
          }}
          onSave={(platformId, model) =>
            setPortalLinkAi(aiLink.id, platformId, model).then((list) => {
              setPortals(list)
            })
          }
          onRemove={() =>
            setPortalLinkAi(aiLink.id, '', '').then((list) => {
              setPortals(list)
            })
          }
        />
      )}
    </div>
  )
}
