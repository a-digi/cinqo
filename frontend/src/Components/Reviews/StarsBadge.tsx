import { useEffect, useState } from 'react'
import { summaryBatcher, type BriefAnswer } from './summaryBatcher'
import { StarRating } from './StarRating'
import { briefText, formatAverage, nearestHalf, reviewCountText } from './reviewRules'

interface StarsBadgeProps {
  owner: string
  entityType: string
  entityId: string
}

// The small display for one row of a list: the average in stars with the number and the count beside it. Many of these on one page cost
// one request (see summaryBatcher). If the answer cannot be had it shows nothing rather than an error in the middle of a table.
export function StarsBadge({ owner, entityType, entityId }: StarsBadgeProps) {
  const [answer, setAnswer] = useState<BriefAnswer | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    setAnswer(null)
    setFailed(false)
    summaryBatcher.request(owner, entityType, entityId).then(
      (a) => {
        if (!cancelled) setAnswer(a)
      },
      () => {
        if (!cancelled) setFailed(true)
      },
    )
    return () => {
      cancelled = true
    }
  }, [owner, entityType, entityId])

  if (failed) return null
  if (answer === null) return <span className="inline-block h-4 w-24 animate-pulse rounded bg-gray-100" aria-label="Loading the rating" />
  const { brief, scale } = answer
  if (brief.count === 0) return <span className="text-xs text-gray-400">{reviewCountText(0)}</span>
  return (
    <span className="inline-flex items-center gap-1.5" title={briefText(brief.count, brief.average)}>
      <StarRating rating={nearestHalf(brief.average)} scale={scale} halfStars size="sm" />
      <span className="text-xs tabular-nums text-gray-600">
        {formatAverage(brief.average)} <span className="text-gray-400">({brief.count})</span>
      </span>
    </span>
  )
}
