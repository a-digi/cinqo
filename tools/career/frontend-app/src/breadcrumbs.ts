import {
  APPLICATIONS_PATH,
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  COURSES_ENTRY_PATH,
  COURSES_PATH,
  CRAWLER_SETTINGS_PATH,
  CRAWL_MONITOR_PATH,
  CV_BUILDER_PATH,
  CV_BUILDER_PERSONA_PATH,
  CV_DOCUMENTS_PATH,
  CV_DOCUMENTS_PERSONA_PATH,
  EDUCATION_ENTRY_PATH,
  EDUCATION_PATH,
  EXPERIENCE_ENTRY_PATH,
  EXPERIENCE_PATH,
  EXPERIENCE_PERSONA_PATH,
  FAQ_PATH,
  GOALS_PATH,
  GUIDE_CONTACTS_PATH,
  GUIDE_JOBS_PATH,
  GUIDE_PATH,
  GUIDE_PROFILE_PATH,
  HOBBIES_ENTRY_PATH,
  HOBBIES_PATH,
  IMPORT_CV_PATH,
  IMPORT_CV_RUN_PATH,
  JOBS_PATH,
  JOB_BOARD_PROFILE_PATH,
  JOB_DETAILS_PATH,
  LANGUAGES_ENTRY_PATH,
  LANGUAGES_PATH,
  NEW_COMPANY_PATH,
  NEW_PERSONA_PATH,
  NEW_PROFILE_PATH,
  NEW_RECRUITER_PATH,
  PERSONAS_PATH,
  PERSONAS_PROFILE_PATH,
  PERSONA_DETAILS_ONE_PATH,
  PERSONA_DETAILS_PATH,
  PLATFORMS_PATH,
  PROFILES_PATH,
  PROFILE_IMAGE_PATH,
  PROFILE_IMAGE_PROFILE_PATH,
  PROFILE_SETTINGS_PROFILE_PATH,
  RECRUITERS_PATH,
  SETTINGS_PATH,
  SKILLS_PATH,
  SKILL_PERSONA_PATH,
  VIEW_COMPANY_PATH,
  VIEW_PERSONA_ID_PATH,
  VIEW_PLATFORM_PATH,
  VIEW_PROFILE_PATH,
  VIEW_RECRUITER_PATH,
  experiencePathFor,
  viewPersonaPathFor,
  viewProfilePathFor,
} from './paths'

// The bridge's own types (its interfaces are local to the d.ts file).
type Bridge = Window['__cinqoToolBridge']
type Trail = Parameters<NonNullable<Bridge['registerBreadcrumbs']>>[0]

// The trails the breadcrumb bar shows on the Career pages that sit below another page (plan/ai/frontend/step-270-breadcrumb.md, step 5).
// The words are the menu's (menu.ts) and the pages' own ("Target roles" is what the Persona pages are called); a menu group has no page, so
// it is plain text. A list page the menu opens has no trail. The pages keyed by a persona id (Skills, Experience, CV ...) have none either:
// their address does not say which profile the persona belongs to, so a link up to the profile cannot be built from it (see plan 270).
// A test checks that every path here is a path main.tsx registers a route for.
const PROFILES = { label: 'Profiles', path: PROFILES_PATH }

// One page of a profile whose id is in the address: Profiles › Profile › the page.
function profilePage(path: string, label: string): Trail {
  return {
    path,
    items: ({ params }) => [PROFILES, { label: 'Profile', path: viewProfilePathFor(params.profileId) }, { label }],
  }
}

// One page of a target role whose id is in the address: Profiles › Target role › the page.
function personaPage(path: string, label: string): Trail {
  return {
    path,
    items: ({ params }) => [PROFILES, { label: 'Target role', path: viewPersonaPathFor(params.personaId) }, { label }],
  }
}

// The same profile the entry page was opened for, as the query of its list page ("" when the address names none).
function profileQuery(search: string): string {
  const id = new URLSearchParams(search).get('profileId')
  return id ? `?profileId=${encodeURIComponent(id)}` : ''
}

// An add/edit page of a profile's list (education, languages, courses, hobbies): Profiles › the list (back to the same profile) › Add/Edit.
function entryPage(path: string, listPath: string, list: string, noun: string): Trail {
  return {
    path,
    items: ({ search }) => [
      PROFILES,
      { label: list, path: `${listPath}${profileQuery(search)}` },
      { label: `${new URLSearchParams(search).get('id') ? 'Edit' : 'Add'} ${noun}` },
    ],
  }
}

const JOBS = { label: 'Jobs', path: JOBS_PATH }
const PLATFORMS = { label: 'Platforms', path: PLATFORMS_PATH }
const GUIDE = { label: 'Guide', path: GUIDE_PATH }

export const CAREER_BREADCRUMBS: readonly Trail[] = [
  profilePage(PERSONAS_PROFILE_PATH, 'Target roles'),
  profilePage(JOB_BOARD_PROFILE_PATH, 'Job board'),
  profilePage(PROFILE_SETTINGS_PROFILE_PATH, 'Job settings'),
  { path: VIEW_PROFILE_PATH, items: [PROFILES, { label: 'Profile' }] },
  { path: NEW_PROFILE_PATH, items: [PROFILES, { label: 'New profile' }] },
  { path: VIEW_PERSONA_ID_PATH, items: [PROFILES, { label: 'Target role' }] },
  { path: NEW_PERSONA_PATH, items: [PROFILES, { label: 'New target role' }] },
  { path: IMPORT_CV_RUN_PATH, items: [{ label: 'CV' }, { label: 'Import', path: IMPORT_CV_PATH }, { label: 'Import review' }] },
  { path: JOB_DETAILS_PATH, items: [{ label: 'Jobs', path: JOBS_PATH }, { label: 'Job' }] },
  { path: VIEW_PLATFORM_PATH, items: [{ label: 'Platforms', path: PLATFORMS_PATH }, { label: 'Platform' }] },
  { path: VIEW_COMPANY_PATH, items: [{ label: 'Companies', path: COMPANIES_PATH }, { label: 'Company' }] },
  { path: NEW_COMPANY_PATH, items: [{ label: 'Companies', path: COMPANIES_PATH }, { label: 'New company' }] },
  { path: VIEW_RECRUITER_PATH, items: [{ label: 'Recruiters', path: RECRUITERS_PATH }, { label: 'Recruiter' }] },
  { path: NEW_RECRUITER_PATH, items: [{ label: 'Recruiters', path: RECRUITERS_PATH }, { label: 'New recruiter' }] },
  { path: CLEANUP_SETTINGS_PATH, items: [{ label: 'Settings' }, { label: 'Job cleanup' }] },
  { path: CRAWLER_SETTINGS_PATH, items: [{ label: 'Settings' }, { label: 'Job crawler' }] },
  // Every other page (plan 270, "a trail on all pages"): the menu's own words, and the pages that sit below another one.
  { path: PROFILES_PATH, items: [{ label: 'Profiles' }] },
  profilePage(PROFILE_IMAGE_PROFILE_PATH, 'Photos'),
  { path: PROFILE_IMAGE_PATH, items: [PROFILES, { label: 'Photos' }] },
  { path: PERSONAS_PATH, items: [PROFILES, { label: 'Target roles' }] },
  { path: GOALS_PATH, items: [PROFILES, { label: 'Goals' }] },
  { path: PERSONA_DETAILS_PATH, items: [PROFILES, { label: 'Target role details' }] },
  personaPage(PERSONA_DETAILS_ONE_PATH, 'Details'),
  { path: SKILLS_PATH, items: [PROFILES, { label: 'Skills' }] },
  personaPage(SKILL_PERSONA_PATH, 'Skills'),
  { path: EXPERIENCE_PATH, items: [PROFILES, { label: 'Experience' }] },
  personaPage(EXPERIENCE_PERSONA_PATH, 'Experience'),
  {
    path: EXPERIENCE_ENTRY_PATH,
    items: ({ search }) => {
      const q = new URLSearchParams(search)
      const personaId = q.get('personaId')
      return [
        PROFILES,
        { label: 'Experience', path: personaId ? experiencePathFor(personaId) : EXPERIENCE_PATH },
        { label: `${q.get('id') ? 'Edit' : 'Add'} experience` },
      ]
    },
  },
  { path: EDUCATION_PATH, items: [PROFILES, { label: 'Education' }] },
  entryPage(EDUCATION_ENTRY_PATH, EDUCATION_PATH, 'Education', 'education'),
  { path: LANGUAGES_PATH, items: [PROFILES, { label: 'Languages' }] },
  entryPage(LANGUAGES_ENTRY_PATH, LANGUAGES_PATH, 'Languages', 'language'),
  { path: COURSES_PATH, items: [PROFILES, { label: 'Courses' }] },
  entryPage(COURSES_ENTRY_PATH, COURSES_PATH, 'Courses', 'course'),
  { path: HOBBIES_PATH, items: [PROFILES, { label: 'Hobbies' }] },
  entryPage(HOBBIES_ENTRY_PATH, HOBBIES_PATH, 'Hobbies', 'hobby'),
  { path: CV_DOCUMENTS_PATH, items: [{ label: 'CV' }, { label: 'CVs' }] },
  personaPage(CV_DOCUMENTS_PERSONA_PATH, 'CVs'),
  { path: CV_BUILDER_PATH, items: [{ label: 'CV' }, { label: 'CV builder' }] },
  personaPage(CV_BUILDER_PERSONA_PATH, 'CV builder'),
  { path: IMPORT_CV_PATH, items: [{ label: 'CV' }, { label: 'Import' }] },
  { path: JOBS_PATH, items: [{ label: 'Jobs' }] },
  { path: APPLICATIONS_PATH, items: [JOBS, { label: 'Applications' }] },
  { path: PLATFORMS_PATH, items: [{ label: 'Platforms' }] },
  { path: CRAWL_MONITOR_PATH, items: [PLATFORMS, { label: 'Crawl Monitor' }] },
  { path: COMPANIES_PATH, items: [{ label: 'Companies' }] },
  { path: RECRUITERS_PATH, items: [{ label: 'Recruiters' }] },
  { path: SETTINGS_PATH, items: [{ label: 'Settings' }, { label: 'AI' }] },
  { path: GUIDE_PATH, items: [{ label: 'Guide' }] },
  { path: GUIDE_PROFILE_PATH, items: [GUIDE, { label: 'Profiles' }] },
  { path: GUIDE_JOBS_PATH, items: [GUIDE, { label: 'Jobs' }] },
  { path: GUIDE_CONTACTS_PATH, items: [GUIDE, { label: 'Contacts' }] },
  { path: FAQ_PATH, items: [GUIDE, { label: 'Questions' }] },
]

// Hands the trails to the host. A host older than plan 270 has no registerBreadcrumbs: nothing is registered and nothing breaks.
export function registerCareerBreadcrumbs(bridge: { registerBreadcrumbs?: Bridge['registerBreadcrumbs'] }): void {
  if (typeof bridge.registerBreadcrumbs !== 'function') return
  for (const trail of CAREER_BREADCRUMBS) bridge.registerBreadcrumbs(trail)
}
