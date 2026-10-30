import { useState } from 'react'
import { createProfile } from '../../api'
import { PlusIcon, UserIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PROFILES_PATH } from '../../paths'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'

// The page behind the Profiles list's "New profile" button. Reachable, but menu-less,
// like the other create pages; saving or cancelling goes back to the list.
export function NewProfilePage() {
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [error, setError] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  function backToList() {
    window.__cinqoToolBridge.navigate(PROFILES_PATH)
  }

  function handleCreate() {
    const first = firstName.trim()
    const last = lastName.trim()
    if (!first && !last) {
      setError('First or last name is required')
      return
    }
    setError('')
    setIsSaving(true)
    createProfile(first, last)
      .then(backToList)
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="career-v-stretch max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UserIcon />}
        title="New profile"
        description="A profile is one job seeker. Personas, skills and experience hang off it; you can add links afterwards."
      />
      <form
        onSubmit={(e) => {
          e.preventDefault()
          handleCreate()
        }}
        className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
        style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
      >
        <div className="mb-4 grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="profile-first">
              First name
            </label>
            <input
              id="profile-first"
              // eslint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              value={firstName}
              onChange={(e) => {
                setFirstName(e.target.value)
              }}
              placeholder="First name"
              className={inputClass}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="profile-last">
              Last name
            </label>
            <input
              id="profile-last"
              value={lastName}
              onChange={(e) => {
                setLastName(e.target.value)
              }}
              placeholder="Last name"
              className={inputClass}
            />
          </div>
        </div>
        <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
        <div className="mt-3 flex gap-2">
          <button
            type="submit"
            disabled={isSaving}
            className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
          >
            <PlusIcon />
            Create profile
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
    </div>
  )
}
