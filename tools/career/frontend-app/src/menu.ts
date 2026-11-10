import {
  COMPANIES_PATH,
  CRAWL_MONITOR_PATH,
  CV_DOCUMENTS_PATH,
  FAQ_PATH,
  GUIDE_PATH,
  GUIDE_CONTACTS_PATH,
  GUIDE_JOBS_PATH,
  GUIDE_PROFILE_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  APPLICATIONS_PATH,
  PLATFORMS_PATH,
  PROFILES_PATH,
  RECRUITERS_PATH,
  CLEANUP_SETTINGS_PATH,
  CRAWLER_SETTINGS_PATH,
  SETTINGS_PATH,
} from './paths'

import {
  aiSettingsIcon,
  applicationsIcon,
  companiesIcon,
  contactsGuideIcon,
  contactsIcon,
  crawlMonitorIcon,
  cvDocumentsIcon,
  cvIcon,
  findingJobsIcon,
  gettingStartedIcon,
  importIcon,
  jobCleanupIcon,
  jobCrawlerIcon,
  jobsGroupIcon,
  jobsIcon,
  profilesIcon,
  platformsGroupIcon,
  platformsIcon,
  qaIcon,
  questionsIcon,
  recruitersIcon,
  settingsIcon,
} from './menuIcons'

// The bridge's own menu-entry type (its interface is local to the d.ts file).
type MenuEntry = Parameters<Window['__cinqoToolBridge']['registerMenuEntry']>[0]

// The Career tool's sidebar menu: Profiles first (one entry, not a group: each profile's card on that page opens that profile's own pages,
// Personas, Summary, Goals, Skills, Experience, Education, Languages, Courses and Hobbies, so those are not menu entries, plan 233), then CV (your CVs
// and the CV import: a group of its own next to the profile they are built from), then Jobs (with the platforms that feed it and the contacts
// around it: Companies and Recruiters), then Settings and Q&A. A parent is a pure grouping label, never a page of its own, and a group is only
// shown when at least one of its pages is visible to the user (filterVisible, which works at any depth), so the per-page scopes below are
// all that decide who sees what. Every path is unchanged by the regroupings (plan/ai/tools/career/step-172-menu-contacts-under-jobs.md,
// step-228-cv-menu-under-career.md); the pages themselves stay registered at their addresses.
export const careerMenuEntry: MenuEntry = {
  label: 'Career',
  // A briefcase glyph — built only from the sidebar's own sanitizer
  // allowlist (rect, path, line, plus viewBox/stroke-width). See
  // plan/ai/tools/step-15-menu-icons-for-browser-and-career.md.
  icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><rect x="3" y="7" width="14" height="9" rx="1.5" stroke-width="1.3"/><path d="M7 7V5.5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 13 5.5V7" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><line x1="3" y1="11" x2="17" y2="11" stroke-width="1.3"/></svg>',
  children: [
    { label: 'Profiles', path: PROFILES_PATH, scopes: ['tool:career:profile'], icon: profilesIcon },
    {
      label: 'CV',
      icon: cvIcon,
      children: [
        // Your CVs, built from a persona.
        { label: 'CVs', path: CV_DOCUMENTS_PATH, scopes: ['tool:career:cv_documents'], icon: cvDocumentsIcon },
        // Reads a CV into the profile, persona, skills, experience and education (the page is still headed "Import from CV").
        { label: 'Import', path: IMPORT_CV_PATH, scopes: ['tool:career:cv_import'], icon: importIcon },
      ],
    },
    {
      label: 'Jobs',
      icon: jobsGroupIcon,
      children: [
        { label: 'Jobs', path: JOBS_PATH, scopes: ['tool:career:jobs'], icon: jobsIcon },
        { label: 'Applications', path: APPLICATIONS_PATH, scopes: ['tool:career:jobs'], icon: applicationsIcon },
        {
          // The job portals the jobs are crawled from (formerly "Job Sources").
          label: 'Platforms',
          icon: platformsGroupIcon,
          children: [
            { label: 'Platforms', path: PLATFORMS_PATH, scopes: ['tool:career:portals'], icon: platformsIcon },
            { label: 'Crawl Monitor', path: CRAWL_MONITOR_PATH, scopes: ['tool:career:portals'], icon: crawlMonitorIcon },
          ],
        },
        {
          label: 'Contacts',
          icon: contactsIcon,
          children: [
            { label: 'Companies', path: COMPANIES_PATH, scopes: ['tool:career:companies'], icon: companiesIcon },
            { label: 'Recruiters', path: RECRUITERS_PATH, scopes: ['tool:career:companies'], icon: recruitersIcon },
          ],
        },
      ],
    },
    {
      label: 'Settings',
      icon: settingsIcon,
      children: [
        // The AI platform and model every Career feature uses (crawling, Job Match, CVs).
        { label: 'AI', path: SETTINGS_PATH, scopes: ['tool:career:jobs'], icon: aiSettingsIcon },
        // Delete old jobs and jobs that match no profile, by hand or on a schedule.
        { label: 'Job cleanup', path: CLEANUP_SETTINGS_PATH, scopes: ['tool:career:jobs'], icon: jobCleanupIcon },
        // Which jobs the crawler must not insert (Ignore jobs).
        { label: 'Job crawler', path: CRAWLER_SETTINGS_PATH, scopes: ['tool:career:jobs'], icon: jobCrawlerIcon },
      ],
    },
    {
      // Onboarding (plan 170): text pages that explain how to use the tool. No scopes: they call nothing, so everyone who sees the
      // Career menu sees them.
      label: 'Q&A',
      icon: qaIcon,
      children: [
        { label: 'Getting started', path: GUIDE_PATH, icon: gettingStartedIcon },
        { label: 'Profile & CV', path: GUIDE_PROFILE_PATH, icon: cvIcon },
        { label: 'Finding jobs', path: GUIDE_JOBS_PATH, icon: findingJobsIcon },
        { label: 'Companies & recruiters', path: GUIDE_CONTACTS_PATH, icon: contactsGuideIcon },
        { label: 'Questions', path: FAQ_PATH, icon: questionsIcon },
      ],
    },
  ],
}
