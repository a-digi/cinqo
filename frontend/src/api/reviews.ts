import { apiDelete, apiGet, apiPost, apiPut } from './client'

// The review routes answer in camelCase already (api/src/review/handler), so there is no *Raw mapping layer.
export interface Review {
  id: string
  entityType: string
  entityId: string
  // In stars: 3.5 is three and a half.
  rating: number
  // The scale the rating was given on (the type's scale when it was written).
  scale: number
  // '' when only stars were given.
  body: string
  authorId: string
  // '' when the identity provider gave no name; the screen shows "User".
  authorName: string
  createdAt: string
  // '' until the author changes the review.
  updatedAt: string
  // What the signed-in user may do with this review, decided by the server.
  canEdit: boolean
  canDelete: boolean
}

export interface ReviewSummary {
  // Reviews on the type's current scale: how many and their average (two decimals).
  count: number
  average: number
  // 2 x scale entries: entry i is how many reviews gave (i + 1) / 2 stars.
  distribution: number[]
  // Reviews written on another scale, left out of everything above.
  otherScale: number
}

export interface ReviewPage {
  scale: number
  halfStars: boolean
  summary: ReviewSummary
  // The signed-in user's own review, null when they have none.
  mine: Review | null
  reviews: Review[]
  // Every review of the entity, whatever its scale.
  total: number
  // The id to pass as `before` for the next (older) page; '' when there is none.
  nextBefore: string
  // Whether the signed-in user may give a review here (false for a read-only user).
  canCreate: boolean
}

export interface ReviewsTarget {
  owner: string
  entityType: string
  entityId: string
}

export interface ReviewBrief {
  count: number
  average: number
}

export interface ReviewSummaries {
  scale: number
  halfStars: boolean
  summaries: Record<string, ReviewBrief>
}

function base(t: ReviewsTarget): string {
  return `/api/v1/reviews/${encodeURIComponent(t.owner)}/${encodeURIComponent(t.entityType)}/${encodeURIComponent(t.entityId)}`
}

export async function fetchReviews(t: ReviewsTarget, opts: { limit?: number; before?: string } = {}): Promise<ReviewPage> {
  const q = new URLSearchParams()
  if (opts.limit !== undefined) q.set('limit', String(opts.limit))
  if (opts.before) q.set('before', opts.before)
  const query = q.toString()
  const raw = await apiGet<{ message: ReviewPage }>(`${base(t)}${query ? `?${query}` : ''}`)
  return raw.message
}

// Sets the signed-in user's own review (creates it, or replaces the one they have). The scale and the half stars are the type's, decided
// by the server; only the rating and the optional text are sent.
export async function putReview(t: ReviewsTarget, rating: number, body: string): Promise<Review> {
  const raw = await apiPut<{ message: Review }>(base(t), { rating, body })
  return raw.message
}

// Removes the signed-in user's own review (no error when there is none).
export async function deleteMyReview(t: ReviewsTarget): Promise<void> {
  await apiDelete(base(t))
}

// Removes one review by id: the user's own, or anyone's for an administrator.
export async function deleteReviewById(t: ReviewsTarget, reviewId: string): Promise<void> {
  await apiDelete(`${base(t)}?reviewId=${encodeURIComponent(reviewId)}`)
}

// Count and average for up to 100 entities of one type in a single call (a list shows its stars without one call per row).
export async function fetchReviewSummaries(owner: string, entityType: string, ids: string[]): Promise<ReviewSummaries> {
  const raw = await apiPost<{ message: ReviewSummaries }>(
    `/api/v1/review-summaries/${encodeURIComponent(owner)}/${encodeURIComponent(entityType)}`,
    { ids },
  )
  return raw.message
}
