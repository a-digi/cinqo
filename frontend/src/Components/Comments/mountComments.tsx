import { createRoot, type Root } from 'react-dom/client'
import { CommentsPanel } from './CommentsPanel'
import { mountOptionsProblem, type MountOptions } from './commentRules'

// The panel in each container, with a token that says which mount is current. Mounting
// into a container that already holds a panel re-renders the same root (a second
// createRoot on one container is an error) instead of stacking another.
const mounted = new WeakMap<HTMLElement, { root: Root; token: symbol }>()

// Draws the comments panel into a container a tool's page provides, and returns a
// function that removes it. This is what bridge.mountComments does. The panel is
// its own small React tree (a tool's bundle cannot share the app's), so it needs
// nothing from the app's providers: it talks to the API directly.
export function mountComments(container: HTMLElement, options: MountOptions): () => void {
  const problem = mountOptionsProblem(options)
  if (problem) {
    console.error(`mountComments: ${problem}.`)
    return () => undefined
  }

  const root = mounted.get(container)?.root ?? createRoot(container)
  const token = Symbol('comments-mount')
  mounted.set(container, { root, token })
  // The key restarts the panel's state when it is pointed at a different entity.
  root.render(
    <CommentsPanel
      key={`${options.owner}/${options.entityType}/${options.entityId}`}
      owner={options.owner}
      entityType={options.entityType}
      entityId={options.entityId}
      title={options.title}
    />,
  )

  return () => {
    // Deferred: this is usually called from a page's own cleanup, i.e. while another
    // React tree is committing, and React refuses to unmount a root synchronously then.
    queueMicrotask(() => {
      if (mounted.get(container)?.token !== token) return // replaced by a later mount
      mounted.delete(container)
      root.unmount()
    })
  }
}
