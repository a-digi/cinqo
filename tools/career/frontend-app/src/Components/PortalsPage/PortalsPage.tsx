import { useEffect, useState } from 'react'
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
} from '../../api'
import { fetchPlatforms, fetchPlatformKeys, type Platform } from '../../Cinqo/Platform/platformRepository'
import { CrawlPlatformPicker } from '../../Crawler/CrawlPlatformPicker/CrawlPlatformPicker'
import { AutoDiscoveryPanel } from '../../Crawler/AutoDiscoveryPanel/AutoDiscoveryPanel'
import { CrawlPanel } from '../../Crawler/CrawlPanel/CrawlPanel'
import { PlusIcon } from '../../Shared/Icons/icons'
import { Modal } from '../../Shared/Modal/Modal'
import { AccordionItem } from '../../Shared/Accordion/AccordionItem'

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

  // The shared AI-platform/model choice CrawlPanel's own "Crawl with
  // AI"/"Generate with AI" both need — picked once per page, not per
  // link, so this state lives here rather than in CrawlPanel itself.
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [selectedPlatformId, setSelectedPlatformId] = useState<string | null>(null)
  const [selectedModel, setSelectedModel] = useState<string | null>(null)

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
    // Prefers whichever platform already has at least one key
    // registered — a fresh key list load failure degrades to today's
    // exact "just pick the first platform" behavior rather than
    // blocking selection or surfacing a page-level error, since this
    // is a selection preference, not a correctness requirement. See
    // plan/ai/tools/career/step-61-portals-prefer-platform-with-key.md.
    Promise.all([fetchPlatforms(), fetchPlatformKeys().catch(() => [])])
      .then(([list, keys]) => {
        setPlatforms(list)
        if (list.length > 0) {
          const withKey = list.find((p) => keys.some((k) => k.platform === p.id))
          const chosen = withKey ?? list[0]
          setSelectedPlatformId(chosen.id)
          setSelectedModel(chosen.models.length > 0 ? chosen.models[0] : null)
        }
      })
      .catch(() => {
        // Left empty (no platforms) rather than surfacing this as a
        // page-level error — every "Crawl now" button already
        // disables itself when platforms.length === 0, which is
        // enough signal on its own.
      })
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

  const selectedPlatform = platforms.find((p) => p.id === selectedPlatformId) ?? null

  function handleSelectPlatform(id: string) {
    setSelectedPlatformId(id)
    const next = platforms.find((p) => p.id === id)
    setSelectedModel(next && next.models.length > 0 ? next.models[0] : null)
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
        setAddingLinkPortalId(null)
        load()
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

  return (
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <div className="mb-1.5 flex items-start justify-between gap-3">
        <h1 className="text-xl font-semibold">Portals</h1>
        <button
          type="button"
          onClick={() => {
            setIsNewPortalModalOpen(true)
          }}
          className="flex shrink-0 items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          New portal
        </button>
      </div>
      <p className="mb-5 text-sm text-gray-500">
        Job portals to crawl — each portal owns one or more links, and each link can carry its own YAML crawl instructions (the exact shape
        the browser tool's own crawl_paginated expects), normally prepared by the AI. Deleting a portal permanently deletes every link it
        owns.
      </p>

      <CrawlPlatformPicker
        platforms={platforms}
        selectedPlatformId={selectedPlatformId}
        selectedModel={selectedModel}
        onSelectPlatform={handleSelectPlatform}
        onSelectModel={setSelectedModel}
      />

      <AutoDiscoveryPanel />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {/* items-start: without it, CSS Grid's default align-items:stretch
          forces every card in a row to match the tallest (open) sibling's
          height, leaving ugly empty space at the bottom of closed cards. */}
      <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-2">
        {portals.map((p, index) => (
          <AccordionItem
            key={p.id}
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
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium">{p.name}</span>
                  <span className="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
                    {p.links.length} {p.links.length === 1 ? 'link' : 'links'}
                  </span>
                  <span className="ml-auto flex shrink-0 gap-2">
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        startEditPortal(p)
                      }}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleDeletePortal(p.id)
                      }}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
                    >
                      Delete
                    </button>
                  </span>
                </div>
              )
            }
            // Staggered mount cascade — animation-delay set inline since
            // Tailwind's own utilities have no per-index dynamic value.
            style={{ animationDelay: `${index * 90}ms` }}
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
              {p.links.map((link) => (
                <div key={link.id} className="rounded-lg border border-gray-200 bg-gray-50 p-3 transition-colors hover:border-gray-300">
                  {editingLinkId === link.id ? (
                    <div className="space-y-1.5">
                      <input
                        // Deliberate: entering edit mode should focus the input
                        // immediately, the same convention most inline-rename UIs use.
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus
                        value={editLinkTitle}
                        onChange={(e) => {
                          setEditLinkTitle(e.target.value)
                        }}
                        placeholder="Title, e.g. Software Engineer jobs, Hamburg"
                        className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                      />
                      <input
                        value={editLinkUrl}
                        onChange={(e) => {
                          setEditLinkUrl(e.target.value)
                        }}
                        placeholder="URL"
                        className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            saveEditLink(link.id)
                          }}
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
                          {link.title ?? '(untitled link)'}
                        </a>
                        <div className="truncate text-xs text-gray-500">{link.url}</div>
                        <div className="truncate text-xs text-gray-400">
                          {link.lastCrawledAt ? `Last crawled: ${new Date(link.lastCrawledAt).toLocaleString()}` : 'Never crawled'}
                        </div>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            startEditLink(link)
                          }}
                          className="text-xs text-gray-500 underline hover:text-gray-700"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleRemoveLink(link.id)
                          }}
                          className="text-xs text-red-700 underline hover:text-red-800"
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  )}

                  <CrawlPanel
                    link={link}
                    selectedPlatform={selectedPlatform}
                    selectedModel={selectedModel}
                    hasPlatforms={platforms.length > 0}
                    onReload={load}
                    onError={setError}
                  />
                </div>
              ))}
            </div>

            <div className="mt-2">
              <button
                type="button"
                onClick={() => {
                  setAddingLinkPortalId(p.id)
                }}
                className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
              >
                <PlusIcon />
                Add link
              </button>
            </div>
          </AccordionItem>
        ))}
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
    </div>
  )
}
