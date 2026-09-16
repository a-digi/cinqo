import { useEffect, useState } from 'react'
import {
  fetchProfiles,
  createProfile,
  updateProfile,
  deleteProfile,
  addProfileExternalLink,
  removeProfileExternalLink,
  type Profile,
} from '../../api'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'

// The job seeker layer, one level above Persona (step 10) — the
// natural new entry point now that nothing else (Personas, and
// everything below Personas) can be used without a profile already
// existing. Reuses SkillsPage's own "chips + inline add form" pattern
// directly for external links — the same repeatable, key-like
// collection shape. See
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function ProfilesPage() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [newFirstName, setNewFirstName] = useState('')
  const [newLastName, setNewLastName] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editFirstName, setEditFirstName] = useState('')
  const [editLastName, setEditLastName] = useState('')
  const [linkDrafts, setLinkDrafts] = useState<Record<string, { platform: string; url: string }>>({})
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

  function handleCreate() {
    const firstName = newFirstName.trim()
    const lastName = newLastName.trim()
    if (!firstName && !lastName) {
      setError('First or last name is required')
      return
    }
    setError('')
    createProfile(firstName, lastName)
      .then((result) => {
        setProfiles(result)
        setNewFirstName('')
        setNewLastName('')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

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

  function handleAddLink(profileId: string) {
    const draft = linkDraft(profileId)
    const platform = draft.platform.trim()
    const url = draft.url.trim()
    if (!platform || !url) return
    setError('')
    addProfileExternalLink(profileId, platform, url)
      .then(() => {
        setLinkDraft(profileId, { platform: '', url: '' })
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

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Profiles</h1>
      <p className="mb-5 text-sm text-gray-500">
        Each profile represents one job seeker using this tool — first name, last name, and external platform links (LinkedIn and so on). A
        profile owns one or more personas. Deleting a profile permanently deletes every persona it owns, and everything those personas own
        in turn (details, skills, experience) — the largest deletion this tool can perform in one call, with no separate confirmation step.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="mb-4 space-y-3">
        {profiles.map((p) => {
          const draft = linkDraft(p.id)
          return (
            <div key={p.id} className="rounded-md border border-gray-200 p-3">
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
                    className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                  />
                  <input
                    value={editLastName}
                    onChange={(e) => {
                      setEditLastName(e.target.value)
                    }}
                    placeholder="Last name"
                    className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
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
                <div className="mb-3 flex items-start justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium">
                      {p.firstName || p.lastName ? `${p.firstName} ${p.lastName}`.trim() : '(unnamed)'}
                    </div>
                    <div className="text-xs text-gray-400">Created {p.createdAt}</div>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        startEdit(p)
                      }}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        handleDelete(p.id)
                      }}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              )}

              <div className="mb-2 flex flex-wrap gap-2">
                {p.externalLinks.map((link) => (
                  <a
                    key={link.platform}
                    href={link.url}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-700 hover:bg-gray-200"
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
              </div>
              <div className="flex gap-2">
                <input
                  value={draft.platform}
                  onChange={(e) => {
                    setLinkDraft(p.id, { ...draft, platform: e.target.value })
                  }}
                  placeholder="Platform, e.g. linkedin"
                  className="w-32 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <input
                  value={draft.url}
                  onChange={(e) => {
                    setLinkDraft(p.id, { ...draft, url: e.target.value })
                  }}
                  placeholder="URL"
                  className="flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <button
                  type="button"
                  onClick={() => {
                    handleAddLink(p.id)
                  }}
                  className="flex items-center gap-1 rounded-md border border-gray-200 px-2.5 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
                >
                  <PlusIcon />
                  Add
                </button>
              </div>
            </div>
          )
        })}
      </div>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">New profile</h2>
        <input
          value={newFirstName}
          onChange={(e) => {
            setNewFirstName(e.target.value)
          }}
          placeholder="First name"
          className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <input
          value={newLastName}
          onChange={(e) => {
            setNewLastName(e.target.value)
          }}
          placeholder="Last name"
          className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={handleCreate}
          className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          Create profile
        </button>
      </section>
    </div>
  )
}
