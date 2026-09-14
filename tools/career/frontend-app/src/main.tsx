// main.tsx is the Career tool's own frontend entry point. Same
// runtime contract browser's own step 15 established: React only for
// the build, one self-contained tools/career/frontend/bundle.js, the
// host provides window.__cinqoToolBridge. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md.
//
// Step 7 split Skills and Experience out of Profile into their own
// pages/routes/menu children. Step 9 added Personas. Step 11 added
// Profiles — the new first child, since nothing else (Personas, and
// everything under a Persona) can be used without one existing first
// (step 10's own "forced to be mapped to an existing Profile") — and
// renamed what used to be called "Profile" to "Persona Details",
// matching the backend's own step 10 rename. See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md,
// plan/ai/tools/career/step-09-persona-frontend.md,
// plan/ai/tools/career/step-10-job-seeker-profile.md, and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
import { createRoot, type Root } from 'react-dom/client'
import { ProfilesPage } from './ProfilesPage'
import { PersonasPage } from './PersonasPage'
import { PersonaDetailsPage } from './PersonaDetailsPage'
import { SkillsPage } from './SkillsPage'
import { ExperiencePage } from './ExperiencePage'
import { JobsPage } from './JobsPage'
import './index.css'

const PROFILES_PATH = '/tools/career/profiles'
const PERSONAS_PATH = '/tools/career/personas'
const PERSONA_DETAILS_PATH = '/tools/career/persona-details'
const SKILLS_PATH = '/tools/career/skills'
const EXPERIENCE_PATH = '/tools/career/experience'
const JOBS_PATH = '/tools/career/jobs'

window.__cinqoToolBridge.registerMenuEntry({
  label: 'Career',
  children: [
    {
      label: 'Profiles',
      path: PROFILES_PATH,
      scopes: ['tool:career:profile'],
    },
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
    {
      label: 'Jobs',
      path: JOBS_PATH,
      scopes: ['tool:career:jobs'],
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
  path: PROFILES_PATH,
  mount: (container) => mountReact(container, <ProfilesPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PERSONAS_PATH,
  mount: (container) => mountReact(container, <PersonasPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PERSONA_DETAILS_PATH,
  mount: (container) => mountReact(container, <PersonaDetailsPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: SKILLS_PATH,
  mount: (container) => mountReact(container, <SkillsPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: EXPERIENCE_PATH,
  mount: (container) => mountReact(container, <ExperiencePage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: JOBS_PATH,
  mount: (container) => mountReact(container, <JobsPage />),
  unmount: unmountReact,
})
