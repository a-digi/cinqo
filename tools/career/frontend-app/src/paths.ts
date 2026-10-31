// Every page path of the Career tool, shared by the route registration (main.tsx)
// and the menu (menu.ts) so the two cannot drift apart.
export const IMPORT_CV_PATH = '/tools/career/import-cv'
export const PROFILES_PATH = '/tools/career/profiles'
export const VIEW_PROFILE_PATH = '/tools/career/profiles/view'
export const PERSONAS_PATH = '/tools/career/personas'
export const PERSONA_DETAILS_PATH = '/tools/career/persona-details'
export const VIEW_PERSONA_PATH = '/tools/career/personas/view'
export const SKILLS_PATH = '/tools/career/skills'
// One persona's skills on their own page: a pattern route, the id arrives as the page's route parameter.
export const SKILL_PERSONA_PATH = '/tools/career/skills/:personaId'
// The address of one persona's skills page.
export function skillsPathFor(personaId: string): string {
  return `${SKILLS_PATH}/${encodeURIComponent(personaId)}`
}
export const EXPERIENCE_PATH = '/tools/career/experience'
export const PROFILE_IMAGE_PATH = '/tools/career/profile-image'
export const CV_DOCUMENTS_PATH = '/tools/career/cv-documents'
export const CV_BUILDER_PATH = '/tools/career/cv-builder'
export const JOBS_PATH = '/tools/career/jobs'
export const JOB_DETAILS_PATH = '/tools/career/job-details'
export const COMPANIES_PATH = '/tools/career/companies'
export const RECRUITERS_PATH = '/tools/career/recruiters'
export const PORTALS_PATH = '/tools/career/portals'
export const CRAWL_MONITOR_PATH = '/tools/career/crawl-monitor'
export const SETTINGS_PATH = '/tools/career/settings'
export const NEW_COMPANY_PATH = '/tools/career/companies/new'
export const NEW_RECRUITER_PATH = '/tools/career/recruiters/new'
export const VIEW_COMPANY_PATH = '/tools/career/companies/view'
export const VIEW_PORTAL_PATH = '/tools/career/portals/view'
export const NEW_PROFILE_PATH = '/tools/career/profiles/new'
export const NEW_PERSONA_PATH = '/tools/career/personas/new'
export const EXPERIENCE_ENTRY_PATH = '/tools/career/experience/entry'
