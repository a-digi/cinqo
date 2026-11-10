import { AppLink } from '../../Shared/AppLink/AppLink'
import { profileCardPathFor } from '../../paths'

// The back link at the top of a profile's own pages (Personas, Summary, Goals, Skills, Experience, Education, Languages, Courses, Hobbies;
// plan 233): it returns to that profile on the Profiles page (its card is scrolled into view and highlighted), not to a list of the page's own
// kind.
export function BackToProfile({ profileId }: { profileId: string }) {
  return (
    <AppLink href={profileCardPathFor(profileId)} className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline">
      ← Profiles
    </AppLink>
  )
}
