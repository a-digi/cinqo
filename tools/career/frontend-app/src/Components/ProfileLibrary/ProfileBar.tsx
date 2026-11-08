import type { Persona, Profile } from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { personasOfProfile } from './libraryRules'

// What sits above a library page: the profile it shows (a dropdown, only when there is more than one) and the profile's personas as
// links to their own pages, where what each persona uses is shown.
export function ProfileBar({
  profiles,
  personas,
  profileId,
  onSelect,
  personaHref,
  personaLabel,
}: {
  profiles: Profile[]
  personas: Persona[]
  profileId: string
  onSelect: (id: string) => void
  personaHref: (personaId: string) => string
  personaLabel: string
}) {
  const mine = personasOfProfile(personas, profileId)
  return (
    <div className="mb-4 space-y-3">
      {profiles.length > 1 && (
        <Dropdown
          label="Profile"
          placeholder="Choose a profile"
          options={profiles.map((p) => ({ value: p.id, label: profileLabel(p) }))}
          value={profileId}
          onChange={onSelect}
        />
      )}
      {mine.length > 0 && (
        <p className="flex flex-wrap items-center gap-2 text-xs text-gray-500">
          <span className="font-medium text-gray-700">{personaLabel}</span>
          {mine.map((p) => (
            <a
              key={p.id}
              href={personaHref(p.id)}
              onClick={(e) => {
                e.preventDefault()
                window.__cinqoToolBridge.navigate(personaHref(p.id))
              }}
              className="rounded-full border border-blue-200 px-2.5 py-0.5 text-blue-700 hover:bg-blue-50"
            >
              {p.name}
            </a>
          ))}
        </p>
      )}
    </div>
  )
}
