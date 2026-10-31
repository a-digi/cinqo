import { useEffect, useState } from 'react'
import { fetchProfiles, updateProfile, deleteProfile, addProfileExternalLink, removeProfileExternalLink, type Profile } from '../../api'
import { CameraIcon, PlusIcon, UserIcon, XIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { NEW_PROFILE_PATH, VIEW_PROFILE_PATH, photosPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { CompletenessMeter } from './CompletenessMeter'
import { PhotoAvatar } from '../ProfileImage/PhotoAvatar'
import { PlatformShelf } from './PlatformShelf'
import { averageScore, firstWithoutLinks, platformCounts } from './profileRules'
import { summarizeProfiles } from './profileStats'
import { PageTips } from '../../Shared/PageTips/PageTips'

// The job seeker layer, one level above Persona (step 10) — the
// natural new entry point now that nothing else (Personas, and
// everything below Personas) can be used without a profile already
// existing. Reuses SkillsPage's own "chips + inline add form" pattern
// directly for external links — the same repeatable, key-like
// collection shape. Header card, an overview (the platform shelf) on
// above, the profiles below; "New profile" opens its own page. See
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function ProfilesPage() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editFirstName, setEditFirstName] = useState('')
  const [editLastName, setEditLastName] = useState('')
  const [linkDrafts, setLinkDrafts] = useState<Record<string, { platform: string; url: string }>>({})
  // The profiles whose add-link form is open; every form starts hidden.
  const [openLinkFormIds, setOpenLinkFormIds] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchProfiles()
      .then(setProfiles)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
  }, [])

  function startEdit(p: Profile) {
    setEditingId(p.id)
    setEditFirstName(p.firstName)
    setEditLastName(p.lastName)
  }

  function cancelEdit() {
    setEditingId(null)
    setEditFirstName('')
    setEditLastName('')
  }

  function saveEdit(id: string) {
    setError('')
    updateProfile(id, { firstName: editFirstName.trim(), lastName: editLastName.trim() })
      .then((result) => {
        setProfiles(result)
        cancelEdit()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    deleteProfile(id)
      .then(() => {
        setProfiles((prev) => prev.filter((p) => p.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function linkDraft(profileId: string) {
    return linkDrafts[profileId] ?? { platform: '', url: '' }
  }

  function setLinkDraft(profileId: string, draft: { platform: string; url: string }) {
    setLinkDrafts((prev) => ({ ...prev, [profileId]: draft }))
  }

  function setLinkFormOpen(profileId: string, open: boolean) {
    setOpenLinkFormIds((prev) => {
      const next = new Set(prev)
      if (open) next.add(profileId)
      else next.delete(profileId)
      return next
    })
  }

  function handleAddLink(profileId: string) {
    const draft = linkDraft(profileId)
    const platform = draft.platform.trim()
    const url = draft.url.trim()
    if (!platform || !url) return
    setError('')
    addProfileExternalLink(profileId, platform, url)
      .then(() => {
        setLinkDraft(profileId, { platform: '', url: '' })
        setLinkFormOpen(profileId, false)
        load()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleRemoveLink(profileId: string, platform: string) {
    setError('')
    removeProfileExternalLink(profileId, platform)
      .then(load)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const summary = summarizeProfiles(profiles)
  const platforms = platformCounts(profiles)

  // Scrolls to the first profile without any link, so the shelf's dashed card leads to work.
  function showFirstWithoutLinks() {
    const id = firstWithoutLinks(profiles)
    if (id) document.getElementById(`profile-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  return (
    <div className="career-v-pop career-a-glow career-t-type max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UserIcon />}
        title="Profiles"
        description="One profile per job seeker, with links to their platforms. Personas, skills and experience belong to a profile."
        pill={{ text: plural(summary.profiles, 'profile', 'profiles') }}
      />
      {/* Steps 105 to 108: the tips sit in a column of their own beside the page (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <PlatformShelf
            platforms={platforms}
            withoutLinks={summary.withoutLinks}
            average={averageScore(profiles)}
            onShowWithoutLinks={showFirstWithoutLinks}
          />

          <div>
            <div className="min-w-0">
              <div className="mb-3 flex items-center justify-start gap-3">
                <button
                  type="button"
                  onClick={() => {
                    window.__cinqoToolBridge.navigate(NEW_PROFILE_PATH)
                  }}
                  className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
                >
                  <PlusIcon />
                  New profile
                </button>
                <p className="min-w-0 flex-1 text-xs text-gray-500">
                  Deleting a profile permanently deletes its personas and everything they own (details, skills, experience), with no further
                  confirmation.
                </p>
              </div>

              <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

              {profiles.length === 0 && !error && (
                <div
                  className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
                  style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
                >
                  No profiles yet. Create the first one with “New profile”.
                </div>
              )}

              <div className="career-stagger space-y-3">
                {profiles.map((p) => {
                  const draft = linkDraft(p.id)
                  const name = profileLabel(p)
                  const linkFormOpen = openLinkFormIds.has(p.id) && editingId !== p.id
                  return (
                    <div
                      key={p.id}
                      id={`profile-${p.id}`}
                      className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                      style={{
                        borderColor: PAGE_ACCENT.soft,
                        background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)`,
                      }}
                    >
                      {editingId === p.id ? (
                        <div className="mb-3">
                          <input
                            // Deliberate: entering edit mode should focus the input
                            // immediately, the same convention most inline-rename UIs use.
                            // eslint-disable-next-line jsx-a11y/no-autofocus
                            autoFocus
                            value={editFirstName}
                            onChange={(e) => {
                              setEditFirstName(e.target.value)
                            }}
                            placeholder="First name"
                            className={`mb-2 ${inputClass}`}
                          />
                          <input
                            value={editLastName}
                            onChange={(e) => {
                              setEditLastName(e.target.value)
                            }}
                            placeholder="Last name"
                            className={`mb-2 ${inputClass}`}
                          />
                          <div className="flex gap-2">
                            <button
                              type="button"
                              onClick={() => {
                                saveEdit(p.id)
                              }}
                              className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                            >
                              Save
                            </button>
                            <button
                              type="button"
                              onClick={cancelEdit}
                              className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                            >
                              Cancel
                            </button>
                          </div>
                        </div>
                      ) : (
                        <div className="mb-3 flex items-start gap-3">
                          {p.imageMediaFileId ? (
                            <PhotoAvatar profileId={p.id} mediaFileId={p.imageMediaFileId} onChanged={load} onError={setError} />
                          ) : (
                            <span
                              className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-bold text-white shadow-sm"
                              style={{ background: accentFor(name).to }}
                              aria-hidden="true"
                            >
                              {initialOf(name)}
                            </span>
                          )}
                          <div className="min-w-0 flex-1">
                            <div className="text-sm font-semibold">{p.firstName || p.lastName ? name : '(unnamed)'}</div>
                            <div className="text-xs text-gray-400">Created {formatDate(p.createdAt)}</div>
                          </div>
                          <div className="flex shrink-0 gap-2">
                            <button
                              type="button"
                              onClick={() => {
                                window.__cinqoToolBridge.navigate(`${VIEW_PROFILE_PATH}?id=${encodeURIComponent(p.id)}`)
                              }}
                              className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                            >
                              View
                            </button>
                            {/* The photo belongs to the profile: its photo page is its own address. */}
                            <button
                              type="button"
                              onClick={() => {
                                window.__cinqoToolBridge.navigate(photosPathFor(p.id))
                              }}
                              className="flex items-center gap-1 rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                            >
                              <CameraIcon className="h-3 w-3" />
                              Photo
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                startEdit(p)
                              }}
                              className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                            >
                              Edit
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                handleDelete(p.id)
                              }}
                              className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                            >
                              Delete
                            </button>
                          </div>
                        </div>
                      )}

                      <CompletenessMeter completeness={p.completeness} />
                      <div className="mb-2 flex flex-wrap gap-2">
                        {p.externalLinks.map((link) => (
                          <a
                            key={link.platform}
                            href={link.url}
                            target="_blank"
                            rel="noreferrer"
                            className="flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium hover:underline"
                            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                          >
                            {link.platform}
                            <button
                              type="button"
                              aria-label={`Remove ${link.platform}`}
                              onClick={(e) => {
                                e.preventDefault()
                                handleRemoveLink(p.id, link.platform)
                              }}
                              className="text-gray-400 hover:text-red-700"
                            >
                              <XIcon />
                            </button>
                          </a>
                        ))}
                        <button
                          type="button"
                          aria-expanded={linkFormOpen}
                          aria-controls={`link-form-${p.id}`}
                          onClick={() => {
                            setLinkFormOpen(p.id, !openLinkFormIds.has(p.id))
                          }}
                          className={`flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-medium ${
                            p.externalLinks.length === 0
                              ? 'border-amber-300 bg-amber-50 text-amber-800 hover:bg-amber-100'
                              : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'
                          }`}
                        >
                          <PlusIcon />
                          Platform
                        </button>
                      </div>
                      {linkFormOpen && (
                        <div id={`link-form-${p.id}`} className="flex gap-2">
                          <input
                            value={draft.platform}
                            onChange={(e) => {
                              setLinkDraft(p.id, { ...draft, platform: e.target.value })
                            }}
                            placeholder="Platform, e.g. linkedin"
                            className="w-32 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                          />
                          <input
                            value={draft.url}
                            onChange={(e) => {
                              setLinkDraft(p.id, { ...draft, url: e.target.value })
                            }}
                            placeholder="URL"
                            className="flex-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                          />
                          <button
                            type="button"
                            onClick={() => {
                              handleAddLink(p.id)
                            }}
                            className="flex items-center gap-1 rounded-md border border-gray-200 bg-white px-2.5 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
                          >
                            <PlusIcon />
                            Add
                          </button>
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          </div>
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="profiles" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}

// The created time as a local date, or as given when it is not one.
function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}
