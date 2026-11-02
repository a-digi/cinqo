import {
  COMPANIES_PATH,
  CRAWL_MONITOR_PATH,
  CV_DOCUMENTS_PATH,
  EDUCATION_PATH,
  EXPERIENCE_PATH,
  FAQ_PATH,
  GUIDE_PATH,
  GUIDE_CONTACTS_PATH,
  GUIDE_JOBS_PATH,
  GUIDE_PROFILE_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  PERSONAS_PATH,
  GOALS_PATH,
  PERSONA_DETAILS_PATH,
  PLATFORMS_PATH,
  PROFILES_PATH,
  RECRUITERS_PATH,
  CLEANUP_SETTINGS_PATH,
  CRAWLER_SETTINGS_PATH,
  SETTINGS_PATH,
  SKILLS_PATH,
} from './paths'

// The bridge's own menu-entry type (its interface is local to the d.ts file).
type MenuEntry = Parameters<Window['__cinqoToolBridge']['registerMenuEntry']>[0]

// The Career tool's sidebar menu: My Profile first (it holds the one-time setup and the CV pages, and everything else works from it),
// then Jobs (with the platforms that feed it and the contacts around it: Companies and Recruiters), then Settings and Q&A. A parent is
// a pure grouping label, never a page of its own, and a group is only shown when at least one of its pages is visible to the user
// (filterVisible, which works at any depth), so the per-page scopes below are all that decide who sees what. Every path is unchanged
// from before the regrouping (plan/ai/tools/career/step-172-menu-contacts-under-jobs.md).
export const careerMenuEntry: MenuEntry = {
  label: 'Career',
  // A briefcase glyph — built only from the sidebar's own sanitizer
  // allowlist (rect, path, line, plus viewBox/stroke-width). See
  // plan/ai/tools/step-15-menu-icons-for-browser-and-career.md.
  icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><rect x="3" y="7" width="14" height="9" rx="1.5" stroke-width="1.3"/><path d="M7 7V5.5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 13 5.5V7" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><line x1="3" y1="11" x2="17" y2="11" stroke-width="1.3"/></svg>',
  children: [
    {
      label: 'My Profile',
      children: [
        { label: 'Profiles', path: PROFILES_PATH, scopes: ['tool:career:profile'] },
        { label: 'Personas', path: PERSONAS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Summary', path: PERSONA_DETAILS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Goals', path: GOALS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Skills', path: SKILLS_PATH, scopes: ['tool:career:profile'] },
        { label: 'Experience', path: EXPERIENCE_PATH, scopes: ['tool:career:profile'] },
        { label: 'Education', path: EDUCATION_PATH, scopes: ['tool:career:profile'] },
        {
          label: 'CV',
          children: [
            // Your CVs, built from a persona.
            { label: 'CVs', path: CV_DOCUMENTS_PATH, scopes: ['tool:career:cv_documents'] },
            // Reads a CV into the profile, persona, skills and experience (the page is still headed "Import from CV").
            { label: 'Import', path: IMPORT_CV_PATH, scopes: ['tool:career:cv_import'] },
          ],
        },
      ],
    },
    {
      label: 'Jobs',
      children: [
        { label: 'Jobs', path: JOBS_PATH, scopes: ['tool:career:jobs'] },
        {
          // The job portals the jobs are crawled from (formerly "Job Sources").
          label: 'Platforms',
          children: [
            { label: 'Platforms', path: PLATFORMS_PATH, scopes: ['tool:career:portals'] },
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
      ],
    },
    {
      label: 'Settings',
      children: [
        // The AI platform and model every Career feature uses (crawling, Job Match, CVs).
        { label: 'AI', path: SETTINGS_PATH, scopes: ['tool:career:jobs'] },
        // Delete old jobs and jobs that match no profile, by hand or on a schedule.
        { label: 'Job cleanup', path: CLEANUP_SETTINGS_PATH, scopes: ['tool:career:jobs'] },
        // Which jobs the crawler must not insert (Ignore jobs).
        { label: 'Job crawler', path: CRAWLER_SETTINGS_PATH, scopes: ['tool:career:jobs'] },
      ],
    },
    {
      // Onboarding (plan 170): text pages that explain how to use the tool. No scopes: they call nothing, so everyone who sees the
      // Career menu sees them.
      label: 'Q&A',
      children: [
        { label: 'Getting started', path: GUIDE_PATH },
        { label: 'Profile & CV', path: GUIDE_PROFILE_PATH },
        { label: 'Finding jobs', path: GUIDE_JOBS_PATH },
        { label: 'Companies & recruiters', path: GUIDE_CONTACTS_PATH },
        { label: 'Questions', path: FAQ_PATH },
      ],
    },
  ],
}
