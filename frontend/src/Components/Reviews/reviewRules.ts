// Pure rules for the reviews screen: how many stars are filled, what a click or a key does, wording, the distribution bars, text limits
// and the options a tool passes when it mounts the panel. No React, so each can be script-checked on its own. See
// plan/ai/review/review-engine.md.
import type { Review } from '../../api/reviews'

export const MAX_BODY = 2000
export const PAGE_SIZE = 20

export type StarFill = 'full' | 'half' | 'empty'

// The fill of each of the scale's stars for a rating (3.5 of 5: full, full, full, half, empty). A half only shows when the rating has one.
export function starFills(rating: number, scale: number): StarFill[] {
  const n = Number.isFinite(scale) ? Math.max(0, Math.floor(scale)) : 0
  const r = Number.isFinite(rating) ? rating : 0
  return Array.from({ length: n }, (_, i) => (r >= i + 1 ? 'full' : r >= i + 0.5 ? 'half' : 'empty'))
}

// The smallest rating one can give: half a star where halves are allowed, else one star.
export function minRating(halfStars: boolean): number {
  return halfStars ? 0.5 : 1
}

// What clicking at a point of a star gives: star `index` (0-based), and how far across it the pointer is (0 left edge, 1 right edge).
// With halves, the left half of a star is half a star; without, any click is the whole star. A star's width of 0 (nothing measured) counts
// as a click on the whole star.
export function ratingFromPoint(index: number, fraction: number, halfStars: boolean): number {
  if (!halfStars) return index + 1
  return fraction > 0 && fraction <= 0.5 ? index + 0.5 : index + 1
}

// How far across a star (0 to 1) a pointer at x is, given the star's left edge and width; 1 when the width is unknown.
export function fractionAcross(x: number, left: number, width: number): number {
  if (!(width > 0)) return 1
  return Math.min(1, Math.max(0, (x - left) / width))
}

// What a key does to the rating of a star input: arrows step by a half (or a whole) star, Home goes to the smallest, End to the scale.
// null for a key that means nothing here. An unset rating (0) starts at the smallest on an up or right arrow.
export function stepRating(current: number, key: string, scale: number, halfStars: boolean): number | null {
  const step = halfStars ? 0.5 : 1
  const min = minRating(halfStars)
  const clamp = (v: number) => Math.min(scale, Math.max(min, v))
  switch (key) {
    case 'ArrowRight':
    case 'ArrowUp':
      return clamp(current + step)
    case 'ArrowLeft':
    case 'ArrowDown':
      return clamp(current - step)
    case 'Home':
      return min
    case 'End':
      return scale
    default:
      return null
  }
}

// "3.5", "4": a rating without a trailing ".0".
export function formatRating(rating: number): string {
  return Number.isInteger(rating) ? String(rating) : String(Math.round(rating * 100) / 100)
}

// "4.3": an average to one decimal (the server sends two).
export function formatAverage(average: number): string {
  return Number.isFinite(average) ? average.toFixed(1) : '0.0'
}

// An average drawn in stars is shown to the nearest half star, so 4.3 reads as 4.5 stars and 4.2 as 4: the number beside it says the rest.
export function nearestHalf(value: number): number {
  return Math.round(value * 2) / 2
}

export function ratingText(rating: number, scale: number): string {
  return `${formatRating(rating)} out of ${scale} stars`
}

export function reviewCountText(count: number): string {
  if (count <= 0) return 'No reviews yet'
  return count === 1 ? '1 review' : `${count.toLocaleString('en')} reviews`
}

// A short line for the bridge's small display: "4.3 · 12 reviews", or "No reviews yet".
export function briefText(count: number, average: number): string {
  return count > 0 ? `${formatAverage(average)} · ${reviewCountText(count)}` : reviewCountText(0)
}

export function otherScaleNote(n: number): string {
  if (n <= 0) return ''
  return n === 1
    ? '1 older review was given on a different scale and is not in the average.'
    : `${n} older reviews were given on a different scale and are not in the average.`
}

export interface DistributionRow {
  stars: number
  count: number
  // 0 to 1: this row's share of all the reviews counted.
  share: number
}

// One row per whole star, highest first. The server counts per half star (entry i = (i + 1) / 2 stars); a half star belongs to the whole
// star above it (3.5 is in the 4-star row), so a row says "up to this many stars".
export function distributionRows(distribution: number[], scale: number): DistributionRow[] {
  const counts = Array.from({ length: Math.max(0, scale) }, () => 0)
  distribution.forEach((n, i) => {
    const row = Math.min(counts.length - 1, Math.floor(i / 2))
    if (row >= 0 && n > 0) counts[row] += n
  })
  const total = counts.reduce((a, b) => a + b, 0)
  return counts.map((count, i) => ({ stars: i + 1, count, share: total > 0 ? count / total : 0 })).reverse()
}

// Characters, not UTF-16 units, as the server counts them.
export function charsUsed(text: string): number {
  return Array.from(text.trim()).length
}

// Why this text cannot be sent, or null when it can. The text is optional, so an empty one is fine.
export function bodyProblem(text: string): string | null {
  const n = charsUsed(text)
  if (n > MAX_BODY) return `Too long: ${n.toLocaleString('en')} of ${MAX_BODY.toLocaleString('en')} characters.`
  return null
}

export function ratingProblem(rating: number): string | null {
  return rating > 0 ? null : 'Choose a rating first.'
}

// Whether the form differs from the review already saved (so Save is only offered when there is something to save).
export function isChanged(mine: Pick<Review, 'rating' | 'body'> | null, rating: number, body: string): boolean {
  if (rating <= 0) return false
  if (mine === null) return true
  return mine.rating !== rating || mine.body !== body.trim()
}

export function authorLabel(r: Pick<Review, 'authorName'>): string {
  return r.authorName.trim() || 'User'
}

export function isEdited(r: Pick<Review, 'updatedAt'>): boolean {
  return r.updatedAt !== ''
}

// A page of older reviews added after what is shown, without repeating one already there.
export function mergeOlder(existing: Review[], older: Review[]): Review[] {
  const seen = new Set(existing.map((r) => r.id))
  return [...existing, ...older.filter((r) => !seen.has(r.id))]
}

export interface MountOptions {
  owner: string
  entityType: string
  entityId: string
  title?: string
}

const NAME = /^[a-z][a-z0-9_]*$/
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// Why the options a tool passed are unusable, or null when they are fine. The same rules as the server's, so a tool author hears about a
// mistake at once; the server checks them again.
export function mountOptionsProblem(o: Partial<MountOptions> | null | undefined): string | null {
  if (!o || typeof o !== 'object') return 'options are required'
  if (typeof o.owner !== 'string' || o.owner.length > 64 || !NAME.test(o.owner))
    return "owner must be the tool's slug (lower-case letters, digits, underscores)"
  if (typeof o.entityType !== 'string' || o.entityType.length > 40 || !NAME.test(o.entityType))
    return 'entityType must be a lower-case name such as "company"'
  if (typeof o.entityId !== 'string' || !UUID.test(o.entityId)) return 'entityId must be a UUID'
  if (o.title !== undefined && typeof o.title !== 'string') return 'title must be a string'
  return null
}
