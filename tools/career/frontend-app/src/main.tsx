// main.tsx is the Career tool's own frontend entry point. Same
// runtime contract browser's own step 15 established: React only for
// the build, one self-contained tools/career/frontend/bundle.js, the
// host provides window.__cinqoToolBridge. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md.
//
// Step 7 split Skills and Experience out of Profile into their own
// pages/routes/menu children. Step 9 added Personas — the new first
// child, since nothing else can be used without one existing first
// (step 8's own "forced to be mapped to an existing Persona"). See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md
// and plan/ai/tools/career/step-09-persona-frontend.md.
import { createRoot, type Root } from 'react-dom/client'
import { PersonasPage } from './PersonasPage'
import { ProfilePage } from './ProfilePage'
import { SkillsPage } from './SkillsPage'
import { ExperiencePage } from './ExperiencePage'
import { JobsPage } from './JobsPage'
import './index.css'

const PERSONAS_PATH = '/tools/career/personas'
const PROFILE_PATH = '/tools/career/profile'
const SKILLS_PATH = '/tools/career/skills'
const EXPERIENCE_PATH = '/tools/career/experience'
const JOBS_PATH = '/tools/career/jobs'

window.__cinqoToolBridge.registerMenuEntry({
  label: 'Career',
  children: [
    {
      label: 'Personas',
      path: PERSONAS_PATH,
      scopes: ['tool:career:profile'],
    },
    {
      label: 'Profile',
      path: PROFILE_PATH,
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
  path: PERSONAS_PATH,
  mount: (container) => mountReact(container, <PersonasPage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: PROFILE_PATH,
  mount: (container) => mountReact(container, <ProfilePage />),
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
