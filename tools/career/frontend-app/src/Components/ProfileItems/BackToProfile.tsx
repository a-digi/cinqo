import type { Profile } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { viewProfilePathFor } from '../../paths'
import { profileLabel } from '../ProfileSwitcher/profileLabel'

// The back link at the top of a profile's own pages (Personas, Summary, Goals, Skills, Experience, Education, Languages, Courses, Hobbies;
// plan 233): it returns to the View page of that profile, named after it ("← Ada Lovelace"), like the Settings page's back link, not to a list of the
// page's own kind. The name comes from the profiles the page already has (or is given); until it is known the link says "← Profile".
export function BackToProfile({ profileId, profiles, name }: { profileId: string; profiles?: Profile[]; name?: string }) {
  const found = profiles?.find((p) => p.id === profileId)
  const label = name !== undefined && name !== '' ? name : found ? profileLabel(found) : ''
  return (
    <AppLink href={viewProfilePathFor(profileId)} className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline">
      {label !== '' ? `← ${label}` : '← Profile'}
    </AppLink>
  )
}
