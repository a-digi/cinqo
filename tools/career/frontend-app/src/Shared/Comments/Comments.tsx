import { useEffect, useMemo, useRef } from 'react'
import type { CommentType } from './commentTypes'

// Career's one comments component. The panel itself (list, add, edit, reply, delete, filters) is drawn
// by the host through the bridge; this only says which entity it is about, so the same component
// serves every page: <Comments entityType={COMMENT_TYPE_JOB} entityId={job.id} />, in a page or inside
// the shared Modal. The type must be declared in manifest.json's "comments" (commentTypes.ts spells
// them), with a read and a write scope. On a host too old to have the comments feature there is no
// bridge method and nothing is drawn.
//
// It carries its own room below the panel (not in a modal, see inModal): the app's floating chat button sits over the bottom corner
// of the page, and a reply box opened on the last comment needs space to scroll into view above it.
export function Comments({
  entityType,
  entityId,
  title = 'Comments',
  inModal = false,
  metadata,
}: {
  entityType: CommentType
  entityId: string
  title?: string
  // Scopes the panel to comments that carry this metadata (step 153), e.g. { profileId }: new comments are written with it and only
  // comments that have it are listed. Leave it out for every comment. A host older than this option ignores it.
  metadata?: Record<string, string>
  // In a modal the room below is not needed (the modal covers the chat button and scrolls itself).
  inModal?: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  // A string, so the panel is mounted again only when the scope really changes, not when the caller builds an equal object again.
  const scopeKey = metadata ? JSON.stringify(Object.entries(metadata).sort()) : ''
  const scope = useMemo<Record<string, string> | undefined>(
    () => (scopeKey === '' ? undefined : Object.fromEntries(JSON.parse(scopeKey) as [string, string][])),
    [scopeKey],
  )

  useEffect(() => {
    const container = ref.current
    const bridge = window.__cinqoToolBridge
    if (!container || typeof bridge.mountComments !== 'function') return
    return bridge.mountComments(container, { owner: 'career', entityType, entityId, title, ...(scope ? { metadata: scope } : {}) })
  }, [entityType, entityId, title, scope])

  return (
    <div className={inModal ? '' : 'mt-6 pb-24'}>
      <div ref={ref} />
    </div>
  )
}
