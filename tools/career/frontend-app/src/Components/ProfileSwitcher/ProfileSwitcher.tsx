import { useEffect, useState } from 'react'
import { fetchProfiles, type Profile } from '../../api'
import { getProfileIdFromLocalStorage, setProfileIdInLocalStorage } from './repositoryLocalStorage'
import { Dropdown } from '../Dropdown/Dropdown'

const PROFILES_PATH = '/tools/career/profiles'

// Used by PersonasPage — resolves an "effective" profile id (the
// stored one if it's still valid, else the first profile in the
// list, else null if there are none at all) and reports it back via
// onChange, which is what actually scopes that page's own persona
// list/create form. Mirrors PersonaSwitcher's exact shape. See
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function ProfileSwitcher({
  profileId,
  onChange,
}: {
  profileId: string | null
  onChange: (id: string) => void
}) {
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    fetchProfiles()
      .then((list) => {
        setProfiles(list)
        if (list.length === 0) return
        const stored = getProfileIdFromLocalStorage()
        const effective = list.find((p) => p.id === stored) ?? list[0]
        onChange(effective.id)
      })
      .catch((err: Error) => setError(err.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return <div className="mb-4 text-sm text-red-700">{error}</div>
  }

  if (profiles === null) {
    return null
  }

  if (profiles.length === 0) {
    return (
      <div className="mb-4 rounded-md border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700">
        No profiles yet.{' '}
        <a href={PROFILES_PATH} onClick={navigateToProfiles} className="font-medium underline">
          Create one to get started
        </a>
        .
      </div>
    )
  }

  return (
    <div className="mb-4 flex items-end gap-2 text-sm">
      <Dropdown
        label="Profile"
        options={profiles.map((p) => ({ value: p.id, label: profileLabel(p) }))}
        value={profileId}
        onChange={(id) => {
          setProfileIdInLocalStorage(id)
          onChange(id)
        }}
      />
      <a
        href={PROFILES_PATH}
        onClick={navigateToProfiles}
        className="pb-1 text-gray-500 underline hover:text-gray-700"
      >
        Manage
      </a>
    </div>
  )
}

export function profileLabel(p: Profile): string {
  const name = `${p.firstName} ${p.lastName}`.trim()
  return name || `Profile ${p.id.slice(0, 8)}`
}

function navigateToProfiles(e: React.MouseEvent) {
  e.preventDefault()
  window.__cinqoToolBridge.navigate(PROFILES_PATH)
}
