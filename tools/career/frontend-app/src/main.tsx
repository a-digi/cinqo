// main.tsx is the Career tool's own frontend entry point. Same
// runtime contract browser's own step 15 established: React only for
// the build, one self-contained tools/career/frontend/bundle.js, the
// host provides window.__cinqoToolBridge. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md.
import { createRoot, type Root } from 'react-dom/client'
import { ProfilePage } from './ProfilePage'
import { JobsPage } from './JobsPage'
import './index.css'

const PROFILE_PATH = '/tools/career/profile'
const JOBS_PATH = '/tools/career/jobs'

window.__cinqoToolBridge.registerMenuEntry({
  label: 'Career',
  children: [
    {
      label: 'Profile',
      path: PROFILE_PATH,
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
  path: PROFILE_PATH,
  mount: (container) => mountReact(container, <ProfilePage />),
  unmount: unmountReact,
})

window.__cinqoToolBridge.registerRoute({
  path: JOBS_PATH,
  mount: (container) => mountReact(container, <JobsPage />),
  unmount: unmountReact,
})
