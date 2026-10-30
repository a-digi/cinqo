import {
  COMPANIES_PATH,
  CRAWL_MONITOR_PATH,
  CV_DOCUMENTS_PATH,
  EXPERIENCE_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  PERSONAS_PATH,
  PERSONA_DETAILS_PATH,
  PORTALS_PATH,
  PROFILES_PATH,
  PROFILE_IMAGE_PATH,
  RECRUITERS_PATH,
  SETTINGS_PATH,
  SKILLS_PATH,
} from './paths'

// The bridge's own menu-entry type (its interface is local to the d.ts file).
type MenuEntry = Parameters<Window['__cinqoToolBridge']['registerMenuEntry']>[0]

// The Career tool's sidebar menu, ordered by how it is used: the daily pages first
// (Jobs, CVs), what feeds and surrounds them next (Job Sources, Contacts), and the
// one-time setup last (My Profile). A parent is a pure grouping label, never a
// page of its own, and a group is only shown when at least one of its pages is
// visible to the user (filterVisible), so the per-page scopes below are all that
// decide who sees what. Every path is unchanged from before the regrouping.
export const careerMenuEntry: MenuEntry = {
  label: 'Career',
  // A briefcase glyph — built only from the sidebar's own sanitizer
  // allowlist (rect, path, line, plus viewBox/stroke-width). See
  // plan/ai/tools/step-15-menu-icons-for-browser-and-career.md.
  icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><rect x="3" y="7" width="14" height="9" rx="1.5" stroke-width="1.3"/><path d="M7 7V5.5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 13 5.5V7" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><line x1="3" y1="11" x2="17" y2="11" stroke-width="1.3"/></svg>',
  children: [
    { label: 'Jobs', path: JOBS_PATH, scopes: ['tool:career:jobs'] },
    { label: 'CVs', path: CV_DOCUMENTS_PATH, scopes: ['tool:career:cv_documents'] },
    {
      label: 'Job Sources',
      children: [
        { label: 'Portals', path: PORTALS_PATH, scopes: ['tool:career:portals'] },
        { label: 'Crawl Monitor', path: CRAWL_MONITOR_PATH, scopes: ['tool:career:portals'] },
      ],
    },
    {
      label: 'Contacts',
      children: [
        { label: 'Companies', path: COMPANIES_PATH, scopes: ['tool:career:companies'] },
        { label: 'Recruiters', path: RECRUITERS_PATH, scopes: ['tool:career:companies'] },
      ],
    },
    {
      label: 'My Profile',
      children: [
        { label: 'Profiles', path: PROFILES_PATH, scopes: ['tool:career:profile'] },
        { label: 'Personas', path: PERSONAS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Summary & Goals', path: PERSONA_DETAILS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Skills', path: SKILLS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Experience', path: EXPERIENCE_PATH, scopes: ['tool:career:profile'] },
        { label: 'Photo', path: PROFILE_IMAGE_PATH, scopes: ['tool:career:profile'] },
        // Moved here from CV: it fills the profile, persona, skills and experience.
        { label: 'Import from CV', path: IMPORT_CV_PATH, scopes: ['tool:career:cv_import'] },
      ],
    },
    // The AI platform and model every Career feature uses (crawling, Job Match, CVs).
    { label: 'Settings', path: SETTINGS_PATH, scopes: ['tool:career:jobs'] },
  ],
}
