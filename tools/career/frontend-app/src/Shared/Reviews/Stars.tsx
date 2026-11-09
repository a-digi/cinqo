import { useEffect, useRef } from 'react'
import type { ReviewType } from './reviewTypes'

// The small stars of one entity (the average in stars, the number and how many reviews) for a row of a list: <Stars entityType="company"
// entityId={c.id} />. The host draws it through the bridge; every row of a page shares one request. On a host too old to have the reviews
// feature nothing is drawn.
export function Stars({ entityType, entityId }: { entityType: ReviewType; entityId: string }) {
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const container = ref.current
    const bridge = window.__cinqoToolBridge
    if (!container || typeof bridge.mountStars !== 'function') return
    return bridge.mountStars(container, { owner: 'career', entityType, entityId })
  }, [entityType, entityId])

  return <span ref={ref} className="inline-block" />
}
