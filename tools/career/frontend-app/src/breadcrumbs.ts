import {
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  CRAWLER_SETTINGS_PATH,
  IMPORT_CV_PATH,
  IMPORT_CV_RUN_PATH,
  JOB_BOARD_PROFILE_PATH,
  JOB_DETAILS_PATH,
  JOBS_PATH,
  NEW_COMPANY_PATH,
  NEW_PERSONA_PATH,
  NEW_PROFILE_PATH,
  NEW_RECRUITER_PATH,
  PERSONAS_PROFILE_PATH,
  PLATFORMS_PATH,
  PROFILE_SETTINGS_PROFILE_PATH,
  PROFILES_PATH,
  RECRUITERS_PATH,
  VIEW_COMPANY_PATH,
  VIEW_PERSONA_ID_PATH,
  VIEW_PLATFORM_PATH,
  VIEW_PROFILE_PATH,
  VIEW_RECRUITER_PATH,
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
]

// Hands the trails to the host. A host older than plan 270 has no registerBreadcrumbs: nothing is registered and nothing breaks.
export function registerCareerBreadcrumbs(bridge: { registerBreadcrumbs?: Bridge['registerBreadcrumbs'] }): void {
  if (typeof bridge.registerBreadcrumbs !== 'function') return
  for (const trail of CAREER_BREADCRUMBS) bridge.registerBreadcrumbs(trail)
}
