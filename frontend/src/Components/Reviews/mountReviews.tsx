import { createRoot, type Root } from 'react-dom/client'
import { ReviewsPanel } from './ReviewsPanel'
import { StarsBadge } from './StarsBadge'
import { mountOptionsProblem, type MountOptions } from './reviewRules'

// The component in each container, with a token that says which mount is current. Mounting into a container that already holds one
// re-renders the same root (a second createRoot on one container is an error) instead of stacking another.
const mounted = new WeakMap<HTMLElement, { root: Root; token: symbol }>()

function mountInto(container: HTMLElement, name: string, options: MountOptions, render: (key: string) => React.ReactNode): () => void {
  const problem = mountOptionsProblem(options)
  if (problem) {
    console.error(`${name}: ${problem}.`)
    return () => undefined
  }
  const root = mounted.get(container)?.root ?? createRoot(container)
  const token = Symbol(name)
  mounted.set(container, { root, token })
  // The key restarts the state when it is pointed at a different entity.
  root.render(render(`${options.owner}/${options.entityType}/${options.entityId}`))
  return () => {
    // Deferred: this is usually called from a page's own cleanup, i.e. while another React tree is committing, and React refuses to
    // unmount a root synchronously then.
    queueMicrotask(() => {
      if (mounted.get(container)?.token !== token) return // replaced by a later mount
      mounted.delete(container)
      root.unmount()
    })
  }
}

// Draws the reviews panel into a container a tool's page provides, and returns a function that removes it. This is what
// bridge.mountReviews does.
export function mountReviews(container: HTMLElement, options: MountOptions): () => void {
  return mountInto(container, 'mountReviews', options, (key) => (
    <ReviewsPanel key={key} owner={options.owner} entityType={options.entityType} entityId={options.entityId} title={options.title} />
  ))
}

// Draws the small stars display (average, number, count) for one entity into a container, for a row of a list. This is what
// bridge.mountStars does; the rows of one page share a single request.
export function mountStars(container: HTMLElement, options: MountOptions): () => void {
  return mountInto(container, 'mountStars', options, (key) => (
    <StarsBadge key={key} owner={options.owner} entityType={options.entityType} entityId={options.entityId} />
  ))
}
