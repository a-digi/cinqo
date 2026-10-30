import { useEffect, useState } from 'react'
import { createPersona, fetchProfiles, type Profile } from '../../../api'
import { LayersIcon, PlusIcon } from '../../../Shared/Icons/icons'
import { inputClass } from '../../../Shared/inputClass'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { PERSONAS_PATH, PROFILES_PATH } from '../../../paths'
import { Dropdown } from '../../Dropdown/Dropdown'
import { PAGE_ACCENT } from '../../JobsPage/jobsTheme'
import { profileLabel } from '../../ProfileSwitcher/profileLabel'
import { getProfileIdFromLocalStorage, setProfileIdInLocalStorage } from '../../ProfileSwitcher/repositoryLocalStorage'

// The page behind the Personas list's "New persona" button. A persona must belong to
// an existing profile: ?profileId= pre-selects one (the list passes the profile it is
// showing), else the remembered one, else the first. Menu-less, like the other create pages.
export function NewPersonaPage() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileId, setProfileId] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const [isLoaded, setIsLoaded] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    fetchProfiles()
      .then((list) => {
        setProfiles(list)
        const wanted = new URLSearchParams(window.location.search).get('profileId') ?? getProfileIdFromLocalStorage()
        setProfileId((list.find((p) => p.id === wanted) ?? list.at(0))?.id ?? '')
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [])

  function backToList() {
    window.__cinqoToolBridge.navigate(PERSONAS_PATH)
  }

  function handleCreate() {
    const trimmed = name.trim()
    if (!profileId) {
      setError('Profile is required')
      return
    }
    if (!trimmed) {
      setError('Name is required')
      return
    }
    setError('')
    setIsSaving(true)
    createPersona(profileId, trimmed, description.trim() || undefined)
      .then(() => {
        // The list shows the remembered profile, so it opens on the one just added to.
        setProfileIdInLocalStorage(profileId)
        backToList()
      })
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="career-v-swing career-a-sheen career-t-wave max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<LayersIcon />}
        title="New persona"
        description="A persona is one role this profile is pursuing. It gets its own summary, skills and experience."
      />
      {isLoaded && profiles.length === 0 ? (
        <div
          className="rounded-3xl border p-6 text-sm text-gray-700"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          No profiles yet.{' '}
          <a
            href={PROFILES_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(PROFILES_PATH)
            }}
            className="font-medium underline"
          >
            Create one first
          </a>
          .
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            handleCreate()
          }}
          className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          <div className="mb-4">
            <Dropdown
              label="Profile"
              placeholder="Select a profile"
              options={profiles.map((p) => ({ value: p.id, label: profileLabel(p) }))}
              value={profileId}
              onChange={setProfileId}
            />
          </div>
          <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="persona-name">
            Name
          </label>
          <input
            id="persona-name"
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
            placeholder={'Name, e.g. "Backend Engineer"'}
            className={`mb-4 ${inputClass}`}
          />
          <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="persona-description">
            Description
          </label>
          <input
            id="persona-description"
            value={description}
            onChange={(e) => {
              setDescription(e.target.value)
            }}
            placeholder="Description (optional)"
            className={`mb-4 ${inputClass}`}
          />
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
          <div className="mt-3 flex gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              <PlusIcon />
              Create persona
            </button>
            <button
              type="button"
              onClick={backToList}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
