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
} from './api'
import { PlusIcon } from './icons'

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

  function load() {
    setError('')
    fetchPortals()
      .then(setPortals)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
  }, [])

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
                        <div className="truncate text-xs font-medium text-gray-900">
                          {link.title || '(untitled link)'}
                        </div>
                        <div className="truncate text-xs text-gray-500">{link.url}</div>
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
