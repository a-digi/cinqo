import { useEffect, useRef } from 'react'

// Cinqo's comments panel for one job. The panel itself (list, add, edit, delete,
// filters) is drawn by the host through the bridge; this tool only says which job it
// is about. The "job" type is declared in manifest.json's "comments", with
// tool:career:jobs as both read and write scope. On a host too old to have the
// comments feature there is no bridge method and the page simply has no comments.
export function JobComments({ jobId }: { jobId: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const container = ref.current
    const bridge = window.__cinqoToolBridge
    if (!container || typeof bridge.mountComments !== 'function') return
    return bridge.mountComments(container, { owner: 'career', entityType: 'job', entityId: jobId, title: 'Comments' })
  }, [jobId])

  return <div ref={ref} />
}
