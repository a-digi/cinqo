import { useEffect, useRef } from 'react'
import type { ReviewType } from './reviewTypes'

// Career's one reviews component. The panel itself (the average, how the ratings are spread, the person's own stars and optional text,
// everyone's reviews) is drawn by the host through the bridge; this only says which entity it is about: <Reviews entityType="company"
// entityId={company.id} />. The type must be declared in manifest.json's "reviews" (reviewTypes.ts spells them), with the scale, whether
// halves are allowed, and a read and a write scope. On a host too old to have the reviews feature there is no bridge method and nothing
// is drawn.
//
// It carries its own room below the panel: the app's floating chat button sits over the bottom corner of the page.
export function Reviews({ entityType, entityId, title = 'Reviews' }: { entityType: ReviewType; entityId: string; title?: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const container = ref.current
    const bridge = window.__cinqoToolBridge
    if (!container || typeof bridge.mountReviews !== 'function') return
    return bridge.mountReviews(container, { owner: 'career', entityType, entityId, title })
  }, [entityType, entityId, title])

  return (
    <div className="mt-6 pb-24">
      <div ref={ref} />
    </div>
  )
}
