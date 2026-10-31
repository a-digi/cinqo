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
import { NewProfilePage } from './Components/ProfilesPage/NewProfilePage'
import { ProfileViewPage } from './Components/ProfilesPage/ProfileViewPage'
import { PersonasPage } from './Components/Persona/PersonasPage/PersonasPage'
import { NewPersonaPage } from './Components/Persona/PersonasPage/NewPersonaPage'
import { PersonaDetailsPage } from './Components/Persona/PersonaDetailsPage/PersonaDetailsPage'
import { SkillsPage } from './Components/SkillsPage/SkillsPage'
import { ExperiencePage } from './Components/ExperiencePage/ExperiencePage'
import { ExperienceEntryPage } from './Components/ExperiencePage/ExperienceEntryPage'
import { ProfileImagePage } from './Components/ProfileImage/ProfileImagePage'
import { CvDocumentsPage } from './Components/CvBuilder/CvDocumentsPage'
import { CvBuilderPage } from './Components/CvBuilder/CvBuilderPage'
import { JobsPage } from './Components/JobsPage/JobsPage'
import { JobDetailsPage } from './Components/JobDetails/JobDetailsPage'
import { CompaniesPage } from './Components/CompaniesPage/CompaniesPage'
import { RecruitersPage } from './Components/RecruitersPage/RecruitersPage'
import { CompanyViewPage } from './Components/CompaniesPage/CompanyViewPage'
import { NewCompanyPage } from './Components/CompaniesPage/NewCompanyPage'
import { NewRecruiterPage } from './Components/RecruitersPage/NewRecruiterPage'
import { PortalsPage } from './Components/PortalsPage/PortalsPage'
import { PortalViewPage } from './Components/PortalsPage/PortalViewPage'
import { CrawlMonitorPage } from './Crawler/CrawlMonitor/CrawlMonitorPage'
import { SettingsPage } from './Components/SettingsPage/SettingsPage'
import { ImportCvPage } from './Components/ImportCv/ImportCvPage'
import './index.css'
import { careerMenuEntry } from './menu'
import {
  IMPORT_CV_PATH,
  PROFILES_PATH,
  PERSONAS_PATH,
  PERSONA_DETAILS_PATH,
  SKILLS_PATH,
  EXPERIENCE_PATH,
  PROFILE_IMAGE_PATH,
  CV_DOCUMENTS_PATH,
  CV_BUILDER_PATH,
  JOBS_PATH,
  JOB_DETAILS_PATH,
  COMPANIES_PATH,
  RECRUITERS_PATH,
  NEW_COMPANY_PATH,
  NEW_PROFILE_PATH,
  NEW_PERSONA_PATH,
  EXPERIENCE_ENTRY_PATH,
  VIEW_COMPANY_PATH,
  VIEW_PROFILE_PATH,
  NEW_RECRUITER_PATH,
  SETTINGS_PATH,
  PORTALS_PATH,
  VIEW_PORTAL_PATH,
  CRAWL_MONITOR_PATH,
} from './paths'

window.__cinqoToolBridge.registerMenuEntry(careerMenuEntry)

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
  path: PROFILE_IMAGE_PATH,
  mount: (container) => {
    mountReact(container, <ProfileImagePage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: CV_DOCUMENTS_PATH,
  mount: (container) => {
    mountReact(container, <CvDocumentsPage />)
  },
  unmount: unmountReact,
})

// No menu entry — reached only via CvDocumentsPage's own "+ New CV"
// (?personaId=) and "Edit" (?id=) actions, same "reachable, but
// menu-less" convention JOB_DETAILS_PATH already established.
window.__cinqoToolBridge.registerRoute({
  path: CV_BUILDER_PATH,
  mount: (container) => {
    mountReact(container, <CvBuilderPage />)
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

// No menu entry — reached only via the lists' own "New company" / "New recruiter"
// buttons, same "reachable, but menu-less" convention JOB_DETAILS_PATH has.
window.__cinqoToolBridge.registerRoute({
  path: EXPERIENCE_ENTRY_PATH,
  mount: (container) => {
    mountReact(container, <ExperienceEntryPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: NEW_PERSONA_PATH,
  mount: (container) => {
    mountReact(container, <NewPersonaPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: NEW_PROFILE_PATH,
  mount: (container) => {
    mountReact(container, <NewProfilePage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: VIEW_PROFILE_PATH,
  mount: (container) => {
    mountReact(container, <ProfileViewPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: VIEW_COMPANY_PATH,
  mount: (container) => {
    mountReact(container, <CompanyViewPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: NEW_COMPANY_PATH,
  mount: (container) => {
    mountReact(container, <NewCompanyPage />)
  },
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: NEW_RECRUITER_PATH,
  mount: (container) => {
    mountReact(container, <NewRecruiterPage />)
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
  path: VIEW_PORTAL_PATH,
  mount: (container) => {
    mountReact(container, <PortalViewPage />)
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

window.__cinqoToolBridge.registerRoute({
  path: SETTINGS_PATH,
  mount: (container) => {
    mountReact(container, <SettingsPage />)
  },
  unmount: unmountReact,
})
