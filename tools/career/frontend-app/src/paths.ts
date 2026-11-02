// Every page path of the Career tool, shared by the route registration (main.tsx)
// and the menu (menu.ts) so the two cannot drift apart.
export const IMPORT_CV_PATH = '/tools/career/import-cv'
// One import run on its own page: a pattern route, the run's id arrives as the route parameter (plan 166).
export const IMPORT_CV_RUN_PATH = '/tools/career/import-cv/:runId'
// The address of one import run.
export function importCvRunPathFor(runId: string): string {
  return `${IMPORT_CV_PATH}/${encodeURIComponent(runId)}`
}
export const PROFILES_PATH = '/tools/career/profiles'
export const VIEW_PROFILE_PATH = '/tools/career/profiles/view'
export const PERSONAS_PATH = '/tools/career/personas'
// One profile's personas on their own page: a pattern route, the id arrives as the page's route parameter. The
// exact paths /personas/new and /personas/view win over it; a profile id is never "new" or "view".
export const PERSONAS_PROFILE_PATH = '/tools/career/personas/:profileId'
// The address of one profile's personas page.
export function personasPathFor(profileId: string): string {
  return `${PERSONAS_PATH}/${encodeURIComponent(profileId)}`
}
export const PERSONA_DETAILS_PATH = '/tools/career/persona-details'
// One persona's summary and goals on its own page: a pattern route, the id arrives as the page's route parameter.
export const PERSONA_DETAILS_ONE_PATH = '/tools/career/persona-details/:personaId'
// The address of one persona's summary and goals page.
export function personaDetailsPathFor(personaId: string): string {
  return `${PERSONA_DETAILS_PATH}/${encodeURIComponent(personaId)}`
}
// The page of one persona: a pattern route, the id arrives as the page's route parameter.
export const VIEW_PERSONA_ID_PATH = '/tools/career/personas/view/:personaId'
// The address of the page of one persona.
export function viewPersonaPathFor(personaId: string): string {
  return `/tools/career/personas/view/${encodeURIComponent(personaId)}`
}
// The earlier address, /personas/view?id=<id>: kept as an exact route that sends on to the address above, so
// links and bookmarks made before keep working.
export const VIEW_PERSONA_PATH = '/tools/career/personas/view'
export const SKILLS_PATH = '/tools/career/skills'
// One persona's skills on their own page: a pattern route, the id arrives as the page's route parameter.
export const SKILL_PERSONA_PATH = '/tools/career/skills/:personaId'
// The address of one persona's skills page.
export function skillsPathFor(personaId: string): string {
  return `${SKILLS_PATH}/${encodeURIComponent(personaId)}`
}
export const EXPERIENCE_PATH = '/tools/career/experience'
// One persona's experience on its own page: a pattern route, the id arrives as the page's route parameter. The
// add/edit page (EXPERIENCE_ENTRY_PATH, an exact path) wins over it; a persona id is never "entry".
export const EXPERIENCE_PERSONA_PATH = '/tools/career/experience/:personaId'
// The address of one persona's experience page.
export function experiencePathFor(personaId: string): string {
  return `${EXPERIENCE_PATH}/${encodeURIComponent(personaId)}`
}
export const JOB_BOARD_PATH = '/tools/career/job-board'
// One profile's job board on its own page: a pattern route, the id arrives as the page's route parameter.
export const JOB_BOARD_PROFILE_PATH = '/tools/career/job-board/:profileId'
// The address of one profile's job board.
export function jobBoardPathFor(profileId: string): string {
  return `${JOB_BOARD_PATH}/${encodeURIComponent(profileId)}`
}
export const PROFILE_SETTINGS_PATH = '/tools/career/profile-settings'
// One profile's auto-add settings on their own page: a pattern route, the id arrives as the page's route parameter.
export const PROFILE_SETTINGS_PROFILE_PATH = '/tools/career/profile-settings/:profileId'
// The address of one profile's auto-add settings (step 140).
export function profileSettingsPathFor(profileId: string): string {
  return `${PROFILE_SETTINGS_PATH}/${encodeURIComponent(profileId)}`
}
export const PROFILE_IMAGE_PATH = '/tools/career/profile-image'
// One profile's photos on their own page: a pattern route, the id arrives as the page's route parameter.
export const PROFILE_IMAGE_PROFILE_PATH = '/tools/career/profile-image/:profileId'
// The address of one profile's photo page.
export function photosPathFor(profileId: string): string {
  return `${PROFILE_IMAGE_PATH}/${encodeURIComponent(profileId)}`
}
export const CV_DOCUMENTS_PATH = '/tools/career/cv-documents'
// One persona's CVs on their own page: a pattern route, the id arrives as the page's route parameter.
export const CV_DOCUMENTS_PERSONA_PATH = '/tools/career/cv-documents/:personaId'
// The address of one persona's CV page.
export function cvDocumentsPathFor(personaId: string): string {
  return `${CV_DOCUMENTS_PATH}/${encodeURIComponent(personaId)}`
}
export const CV_BUILDER_PATH = '/tools/career/cv-builder'
// A new CV for one persona on its own address: a pattern route, the id arrives as the page's route parameter.
export const CV_BUILDER_PERSONA_PATH = '/tools/career/cv-builder/:personaId'
// The address of the builder for a new CV of one persona.
export function cvBuilderPathFor(personaId: string): string {
  return `${CV_BUILDER_PATH}/${encodeURIComponent(personaId)}`
}
export const JOBS_PATH = '/tools/career/jobs'
export const JOB_DETAILS_PATH = '/tools/career/job-details'
export const COMPANIES_PATH = '/tools/career/companies'
export const RECRUITERS_PATH = '/tools/career/recruiters'
export const PLATFORMS_PATH = '/tools/career/platforms'
// The earlier address of the Platforms list (it said portals): kept as a route that sends on to PLATFORMS_PATH.
export const LEGACY_PORTALS_PATH = '/tools/career/portals'
export const CRAWL_MONITOR_PATH = '/tools/career/crawl-monitor'
export const SETTINGS_PATH = '/tools/career/settings'
export const CLEANUP_SETTINGS_PATH = '/tools/career/settings/cleanup'
// The Job crawler's "Ignore jobs" rules (step 150).
export const CRAWLER_SETTINGS_PATH = '/tools/career/settings/crawler'
export const NEW_COMPANY_PATH = '/tools/career/companies/new'
export const NEW_RECRUITER_PATH = '/tools/career/recruiters/new'
// One recruiter on their own page (plan/ai/tools/career/step-168): the id arrives as the ?id= query, like the profile and company pages.
export const VIEW_RECRUITER_PATH = '/tools/career/recruiters/view'
// The address of one recruiter's page.
export function viewRecruiterPathFor(recruiterId: string): string {
  return `${VIEW_RECRUITER_PATH}?id=${encodeURIComponent(recruiterId)}`
}
export const VIEW_COMPANY_PATH = '/tools/career/companies/view'
export const VIEW_PLATFORM_PATH = '/tools/career/platforms/view'
// The earlier address of one platform's page (?id=): kept as a route that sends on to VIEW_PLATFORM_PATH.
export const LEGACY_VIEW_PORTAL_PATH = '/tools/career/portals/view'
export const NEW_PROFILE_PATH = '/tools/career/profiles/new'
export const NEW_PERSONA_PATH = '/tools/career/personas/new'
export const EXPERIENCE_ENTRY_PATH = '/tools/career/experience/entry'
