import { createRoot, type Root } from 'react-dom/client'
import { TerminalViewer } from '../../Shared/Components/Terminal/TerminalViewer'
import { WINDOW_SIZES, type WindowSize } from '../../Shared/Components/Terminal/terminalRules'
import { mountProblem, toolSourceId, type FileReaderMountOptions } from './fileReaderRules'

// The viewer in each container, with a token that says which mount is current. Mounting into a container that
// already holds one re-renders the same root (a second createRoot on one container is an error).
const mounted = new WeakMap<HTMLElement, { root: Root; token: symbol }>()

// Draws the file reader into a container a tool's page provides, and returns a function that removes it. This
// is what bridge.mountFileReader does. Like the comments panel it is its own small React tree (a tool's bundle
// cannot share the app's), so it needs nothing from the app's providers: it talks to the API directly.
export function mountFileReader(container: HTMLElement, options: FileReaderMountOptions): () => void {
  const problem = mountProblem(options)
  if (problem) {
    console.error(`mountFileReader: ${problem}.`)
    return () => undefined
  }

  const root = mounted.get(container)?.root ?? createRoot(container)
  const token = Symbol('file-reader-mount')
  mounted.set(container, { root, token })
  const initialWindow: WindowSize | undefined = WINDOW_SIZES.find((n) => n === options.initialWindow)
  // The key restarts the viewer's state when it is pointed at a different file.
  root.render(
    <TerminalViewer
      key={`${options.tool}/${options.source}/${options.ref}`}
      source={toolSourceId(options.tool, options.source)}
      fileRef={options.ref}
      title={options.title}
      initialWindow={initialWindow}
    />,
  )

  return () => {
    // Deferred: this is usually called from a page's own cleanup, i.e. while another React tree is committing,
    // and React refuses to unmount a root synchronously then.
    queueMicrotask(() => {
      if (mounted.get(container)?.token !== token) return // replaced by a later mount
      mounted.delete(container)
      root.unmount()
    })
  }
}
