// The pages of a profile that its card on the Profiles page opens (plan/ai/tools/career/step-233-profile-cards-open-the-profile-pages.md), in
// the order of the request, each with the sidebar's own icon and the address that shows that one profile's data. Pure, so it can be
// script-checked.
import {
  COURSES_PATH,
  EDUCATION_PATH,
  EXPERIENCE_PATH,
  GOALS_PATH,
  HOBBIES_PATH,
  LANGUAGES_PATH,
  PERSONA_DETAILS_PATH,
  SKILLS_PATH,
  jobBoardPathFor,
  personasPathFor,
} from '../../paths'
import {
  applicationsIcon,
  coursesIcon,
  educationIcon,
  experienceIcon,
  goalsIcon,
  hobbiesIcon,
  languagesIcon,
  personasIcon,
  skillsIcon,
  summaryIcon,
} from '../../menuIcons'
import { buildProfileSearch } from '../ProfileLibrary/libraryRules'
import { applicationsTitle, type ApplicationNumbersView } from './applicationNumbersRules'

export interface ProfileArea {
  key: string
  label: string
  // The menu's icon (raw SVG).
  icon: string
  // The address of the page for one profile.
  href: (profileId: string) => string
}

// One profile's data on a page that takes the profile in its address (?profileId=).
const withProfile = (path: string) => (profileId: string) => `${path}${buildProfileSearch(profileId)}`

export const PROFILE_AREAS: readonly ProfileArea[] = [
  // Personas have a page of their own per profile: the id is part of the path.
  { key: 'personas', label: 'Target roles', icon: personasIcon, href: personasPathFor },
  { key: 'summary', label: 'Summary', icon: summaryIcon, href: withProfile(PERSONA_DETAILS_PATH) },
  { key: 'goals', label: 'Goals', icon: goalsIcon, href: withProfile(GOALS_PATH) },
  { key: 'skills', label: 'Skills', icon: skillsIcon, href: withProfile(SKILLS_PATH) },
  { key: 'experience', label: 'Experience', icon: experienceIcon, href: withProfile(EXPERIENCE_PATH) },
  { key: 'education', label: 'Education', icon: educationIcon, href: withProfile(EDUCATION_PATH) },
  { key: 'languages', label: 'Languages', icon: languagesIcon, href: withProfile(LANGUAGES_PATH) },
  { key: 'courses', label: 'Courses', icon: coursesIcon, href: withProfile(COURSES_PATH) },
  { key: 'hobbies', label: 'Hobbies', icon: hobbiesIcon, href: withProfile(HOBBIES_PATH) },
]

export interface ProfileAreaLink {
  key: string
  label: string
  icon: string
  href: string
  // "Skills of Ada Lovelace", for the tooltip.
  title: string
  // Only on the Applications link (plan 265): what is known of the profile's number of applications, shown as a pill on the tile.
  applications?: ApplicationNumbersView
}

// The links of one profile, named after it: the nine pages of its data, then "Applications" (plan 265), which opens the profile's job board and carries
// the number of its applications when that is known (what the page knows is passed in; without it the tile has no number).
export function profileAreaLinks(
  profileId: string,
  name: string,
  applications: ApplicationNumbersView = { kind: 'unknown' },
): ProfileAreaLink[] {
  const areas = PROFILE_AREAS.map((a) => ({
    key: a.key,
    label: a.label,
    icon: a.icon,
    href: a.href(profileId),
    title: `${a.label} of ${name}`,
  }))
  return [
    ...areas,
    {
      key: 'applications',
      label: 'Applications',
      icon: applicationsIcon,
      href: jobBoardPathFor(profileId),
      title: applicationsTitle(name, applications),
      applications,
    },
  ]
}

// Whether a click on a link should be handled inside the tool (a plain left click); a click with a modifier key, or a middle click, is left to
// the browser (a new tab, a new window, save the link).
export function isPlainLeftClick(e: {
  button: number
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  defaultPrevented: boolean
}): boolean {
  return !e.defaultPrevented && e.button === 0 && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey
}
