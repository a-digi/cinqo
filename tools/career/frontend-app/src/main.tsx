// main.tsx is the Career tool's own frontend entry point. Same
// runtime contract browser's own step 15 established: React only for
// the build, one self-contained tools/career/frontend/bundle.js, the
// host provides window.__cinqoToolBridge. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md.
//
// Step 7 split Skills and Experience out of Profile into their own
// pages/routes/menu children. Step 9 added Personas. Step 11 added
// Profiles — since nothing else (Personas, and everything under a
// Persona) can be used without one existing first (step 10's own
// "forced to be mapped to an existing Profile") — and renamed what
// used to be called "Profile" to "Persona Details", matching the
// backend's own step 10 rename. Step 12 nested Personas/Persona
// Details/Skills/Experience under Profiles itself (Profiles keeps its
// own path — the host's own SidebarMenuItem renders a path and
// children independently — while gaining a caret to expand/collapse
// the four of them); Jobs stays a flat sibling of Profiles, not
// nested, since it's tool-wide rather than persona-scoped. See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md,
// plan/ai/tools/career/step-09-persona-frontend.md,
// plan/ai/tools/career/step-10-job-seeker-profile.md,
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md, and
// plan/ai/tools/career/step-12-nested-profiles-menu.md.
import { createRoot, type Root } from 'react-dom/client'
import { ProfilesPage } from './Components/ProfilesPage/ProfilesPage'
import { PersonasPage } from './Components/Persona/PersonasPage/PersonasPage'
import { PersonaDetailsPage } from './Components/Persona/PersonaDetailsPage/PersonaDetailsPage'
import { SkillsPage } from './Components/SkillsPage/SkillsPage'
import { ExperiencePage } from './Components/ExperiencePage/ExperiencePage'
import { JobsPage } from './Components/JobsPage/JobsPage'
import { JobDetailsPage } from './Components/JobDetails/JobDetailsPage'
import { CompaniesPage } from './Components/CompaniesPage/CompaniesPage'
import { RecruitersPage } from './Components/RecruitersPage/RecruitersPage'
import { PortalsPage } from './Components/PortalsPage/PortalsPage'
import { CrawlMonitorPage } from './Crawler/CrawlMonitor/CrawlMonitorPage'
import { ImportCvPage } from './Components/ImportCv/ImportCvPage'
import './index.css'

const IMPORT_CV_PATH = '/tools/career/import-cv'
const PROFILES_PATH = '/tools/career/profiles'
const PERSONAS_PATH = '/tools/career/personas'
const PERSONA_DETAILS_PATH = '/tools/career/persona-details'
const SKILLS_PATH = '/tools/career/skills'
const EXPERIENCE_PATH = '/tools/career/experience'
const JOBS_PATH = '/tools/career/jobs'
const JOB_DETAILS_PATH = '/tools/career/job-details'
const COMPANIES_PATH = '/tools/career/companies'
const RECRUITERS_PATH = '/tools/career/recruiters'
const PORTALS_PATH = '/tools/career/portals'
const CRAWL_MONITOR_PATH = '/tools/career/crawl-monitor'

window.__cinqoToolBridge.registerMenuEntry({
  label: 'Career',
  // A briefcase glyph — built only from the sidebar's own sanitizer
  // allowlist (rect, path, line, plus viewBox/stroke-width). See
  // plan/ai/tools/step-15-menu-icons-for-browser-and-career.md.
  icon: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor"><rect x="3" y="7" width="14" height="9" rx="1.5" stroke-width="1.3"/><path d="M7 7V5.5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 13 5.5V7" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/><line x1="3" y1="11" x2="17" y2="11" stroke-width="1.3"/></svg>',
  children: [
    {
      label: 'Import CV',
      path: IMPORT_CV_PATH,
      scopes: ['tool:career:cv_import'],
    },
    {
      label: 'Profiles',
      path: PROFILES_PATH,
      scopes: ['tool:career:profile'],
      children: [
        {
          label: 'Personas',
          path: PERSONAS_PATH,
          scopes: ['tool:career:profile'],
        },
        {
          label: 'Persona Details',
          path: PERSONA_DETAILS_PATH,
          scopes: ['tool:career:profile'],
        },
        {
          label: 'Skills',
          path: SKILLS_PATH,
          scopes: ['tool:career:profile'],
        },
        {
          label: 'Experience',
          path: EXPERIENCE_PATH,
          scopes: ['tool:career:profile'],
        },
      ],
    },
    {
      label: 'Companies',
      path: COMPANIES_PATH,
      scopes: ['tool:career:companies'],
    },
    {
      label: 'Recruiters',
      path: RECRUITERS_PATH,
      scopes: ['tool:career:companies'],
    },
    {
      label: 'Jobs',
      path: JOBS_PATH,
      scopes: ['tool:career:jobs'],
      children: [
        {
          label: 'Portals',
          path: PORTALS_PATH,
          scopes: ['tool:career:portals'],
        },
        {
          label: 'Crawl Monitor',
          path: CRAWL_MONITOR_PATH,
          scopes: ['tool:career:portals'],
        },
      ],
    },
  ],
})

const roots = new WeakMap<HTMLElement, Root>()

function mountReact(container: HTMLElement, node: React.ReactNode) {
  const root = createRoot(container)
  roots.set(container, root)
  root.render(node)
}

function unmountReact(container: HTMLElement) {
  roots.get(container)?.unmount()
  roots.delete(container)
}

window.__cinqoToolBridge.registerRoute({
  path: IMPORT_CV_PATH,
  mount: (container) => {
    mountReact(container, <ImportCvPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PROFILES_PATH,
  mount: (container) => {
    mountReact(container, <ProfilesPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PERSONAS_PATH,
  mount: (container) => {
    mountReact(container, <PersonasPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PERSONA_DETAILS_PATH,
  mount: (container) => {
    mountReact(container, <PersonaDetailsPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: SKILLS_PATH,
  mount: (container) => {
    mountReact(container, <SkillsPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: EXPERIENCE_PATH,
  mount: (container) => {
    mountReact(container, <ExperiencePage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: JOBS_PATH,
  mount: (container) => {
    mountReact(container, <JobsPage />)
  },
  unmount: unmountReact,
})

// No menu entry — reached only via JobsPage's own Eye icon, same
// "filtered/detail destination with no sidebar item of its own"
// convention CompaniesPage's "N linked jobs" link already established
// for Jobs itself.
window.__cinqoToolBridge.registerRoute({
  path: JOB_DETAILS_PATH,
  mount: (container) => {
    mountReact(container, <JobDetailsPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: COMPANIES_PATH,
  mount: (container) => {
    mountReact(container, <CompaniesPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: RECRUITERS_PATH,
  mount: (container) => {
    mountReact(container, <RecruitersPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PORTALS_PATH,
  mount: (container) => {
    mountReact(container, <PortalsPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: CRAWL_MONITOR_PATH,
  mount: (container) => {
    mountReact(container, <CrawlMonitorPage />)
  },
  unmount: unmountReact,
})
