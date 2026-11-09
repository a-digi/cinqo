import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../../api/client'
import {
  deleteMyReview,
  deleteReviewById,
  fetchReviews,
  putReview,
  type Review,
  type ReviewPage,
  type ReviewsTarget,
} from '../../api/reviews'
import { useConfirm } from '../../Shared/Components/Modal/useConfirm'
import { accentForAuthor, initialOf, relativeTime } from '../Comments/commentRules'
import { StarRating, STAR_COLOUR } from './StarRating'
import {
  MAX_BODY,
  PAGE_SIZE,
  authorLabel,
  bodyProblem,
  charsUsed,
  distributionRows,
  formatAverage,
  isChanged,
  isEdited,
  mergeOlder,
  nearestHalf,
  otherScaleNote,
  ratingProblem,
  reviewCountText,
} from './reviewRules'

function errorText(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

const inputClass = 'rounded-xl border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-amber-500 focus:outline-none'
const ACCENT = { from: '#f59e0b', to: '#f97316', soft: '#fffbeb', text: '#92400e' }

function Avatar({ authorId, label }: { authorId: string; label: string }) {
  const accent = accentForAuthor(authorId)
  return (
    <span
      className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
      style={{ background: `linear-gradient(135deg, ${accent.from}, ${accent.to})` }}
      aria-hidden="true"
    >
      {initialOf(label)}
    </span>
  )
}

interface PanelProps {
  owner: string
  entityType: string
  entityId: string
  title?: string
}

// The reviews of one entity: the average and how the ratings are spread, the signed-in user's own rating (stars and an optional text), and
// everyone's reviews, newest first. It is its own small React tree (a tool's bundle cannot share the app's), so it asks for nothing from
// the app's providers and talks to the API directly. See plan/ai/review/review-engine.md.
export function ReviewsPanel({ owner, entityType, entityId, title = 'Reviews' }: PanelProps) {
  const target: ReviewsTarget = { owner, entityType, entityId }
  const [page, setPage] = useState<ReviewPage | null>(null)
  const [items, setItems] = useState<Review[]>([])
  const [nextBefore, setNextBefore] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [rating, setRating] = useState(0)
  const [text, setText] = useState('')
  const [now, setNow] = useState(() => new Date())
  const { confirm, dialog } = useConfirm()
  // Only the newest load may write to the screen (a slower, older answer is dropped).
  const loadSeq = useRef(0)

  const load = useCallback(async () => {
    const seq = ++loadSeq.current
    setLoading(true)
    setError('')
    try {
      const p = await fetchReviews({ owner, entityType, entityId }, { limit: PAGE_SIZE })
      if (seq !== loadSeq.current) return
      setPage(p)
      setItems(p.reviews)
      setNextBefore(p.nextBefore)
      setRating(p.mine?.rating ?? 0)
      setText(p.mine?.body ?? '')
    } catch (err) {
      if (seq !== loadSeq.current) return
      setPage(null)
      setItems([])
      setNextBefore('')
      setError(errorText(err, "Couldn't load the reviews."))
    } finally {
      if (seq === loadSeq.current) setLoading(false)
    }
  }, [owner, entityType, entityId])

  useEffect(() => {
    void load()
  }, [load])

  // Keep "5m ago" honest while the page stays open.
  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(new Date())
    }, 60_000)
    return () => {
      window.clearInterval(timer)
    }
  }, [])

  async function loadMore() {
    setLoadingMore(true)
    setError('')
    try {
      const p = await fetchReviews(target, { limit: PAGE_SIZE, before: nextBefore })
      setItems((prev) => mergeOlder(prev, p.reviews))
      setNextBefore(p.nextBefore)
    } catch (err) {
      setError(errorText(err, "Couldn't load more reviews."))
    } finally {
      setLoadingMore(false)
    }
  }

  async function save() {
    if (ratingProblem(rating) !== null || bodyProblem(text) !== null) return
    setBusy(true)
    setError('')
    try {
      await putReview(target, rating, text)
      await load() // the average, the spread and the list all moved: ask again rather than guess
    } catch (err) {
      setError(errorText(err, "Couldn't save your review."))
    } finally {
      setBusy(false)
    }
  }

  async function remove(review: Review, own: boolean) {
    const ok = await confirm({
      title: own ? 'Remove your review' : 'Delete review',
      message: own ? 'Your rating and text will be removed.' : `Delete ${authorLabel(review)}'s review? This cannot be undone.`,
      confirmLabel: own ? 'Remove' : 'Delete',
    })
    if (!ok) return
    setBusy(true)
    setError('')
    try {
      if (own) await deleteMyReview(target)
      else await deleteReviewById(target, review.id)
      await load()
    } catch (err) {
      setError(errorText(err, "Couldn't remove the review."))
    } finally {
      setBusy(false)
    }
  }

  const scale = page?.scale ?? 5
  const halfStars = page?.halfStars ?? false
  const summary = page?.summary
  const mine = page?.mine ?? null
  const problem = bodyProblem(text)
  const used = charsUsed(text)
  const changed = isChanged(mine, rating, text)

  return (
    <section
      className="relative overflow-hidden rounded-3xl border p-5 shadow-sm"
      style={{ borderColor: ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${ACCENT.soft} 100%)` }}
      aria-label={title}
    >
      <div className="relative flex items-center gap-3">
        <span
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl text-white shadow-sm"
          style={{ background: `linear-gradient(135deg, ${ACCENT.from}, ${ACCENT.to})` }}
          aria-hidden="true"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2.5l2.94 6.2 6.8.86-5 4.72 1.3 6.72L12 17.6 5.96 21l1.3-6.72-5-4.72 6.8-.86L12 2.5z" />
          </svg>
        </span>
        <h2 className="min-w-0 flex-1 truncate text-base font-semibold text-gray-900">{title}</h2>
        <span className="rounded-full px-2.5 py-0.5 text-xs font-semibold" style={{ background: ACCENT.soft, color: ACCENT.text }}>
          {page?.total ?? 0}
        </span>
      </div>

      {error && (
        <p role="alert" className="relative mt-3 rounded-xl bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      {loading ? (
        <div className="mt-4 animate-pulse space-y-2" aria-label="Loading reviews">
          <div className="h-20 rounded-2xl bg-white/70" />
          <div className="h-14 rounded-2xl bg-white/70" />
        </div>
      ) : (
        page && (
          <>
            <div className="relative mt-4 flex flex-wrap items-center gap-x-8 gap-y-3 rounded-2xl bg-white/70 p-4">
              {summary && summary.count > 0 ? (
                <>
                  <div className="text-center">
                    <div className="text-4xl font-bold tabular-nums text-gray-900" data-testid="average">
                      {formatAverage(summary.average)}
                    </div>
                    <StarRating rating={nearestHalf(summary.average)} scale={scale} halfStars size="md" />
                    <div className="mt-1 text-xs text-gray-500">{reviewCountText(summary.count)}</div>
                  </div>
                  <ul className="min-w-[12rem] flex-1 space-y-1" aria-label="How the ratings are spread">
                    {distributionRows(summary.distribution, scale).map((row) => (
                      <li key={row.stars} className="flex items-center gap-2 text-xs text-gray-600">
                        <span className="w-4 text-right tabular-nums">{row.stars}</span>
                        <span className="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
                          <span
                            className="block h-full rounded-full"
                            style={{ width: `${Math.round(row.share * 100)}%`, background: STAR_COLOUR }}
                            data-testid={`bar-${row.stars}`}
                          />
                        </span>
                        <span className="w-6 tabular-nums text-gray-500">{row.count}</span>
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <p className="text-sm text-gray-500">{reviewCountText(0)}. {page.canCreate ? 'Be the first to rate.' : ''}</p>
              )}
              {summary && summary.otherScale > 0 && <p className="w-full text-xs text-gray-400">{otherScaleNote(summary.otherScale)}</p>}
            </div>

            {page.canCreate ? (
              <div className="relative mt-4 space-y-2 rounded-2xl bg-white/70 p-4">
                <div className="text-sm font-medium text-gray-900">{mine ? 'Your review' : 'Rate this'}</div>
                <StarRating rating={rating} scale={scale} halfStars={halfStars} size="lg" onChange={setRating} disabled={busy} label="Your rating" />
                <textarea
                  value={text}
                  rows={3}
                  placeholder="Say why (optional)…"
                  aria-label="Your review text"
                  onChange={(e) => {
                    setText(e.target.value)
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                      e.preventDefault()
                      if (!busy && changed && problem === null) void save()
                    }
                  }}
                  className={`w-full ${inputClass}`}
                />
                <div className="flex items-center justify-between gap-3">
                  <span className={`text-xs ${used > MAX_BODY ? 'text-red-600' : 'text-gray-400'}`}>
                    {used.toLocaleString('en')} / {MAX_BODY.toLocaleString('en')}
                  </span>
                  <span className="flex items-center gap-2">
                    {mine && (
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => {
                          void remove(mine, true)
                        }}
                        className="rounded-full border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                      >
                        Remove my review
                      </button>
                    )}
                    <button
                      type="button"
                      disabled={busy || !changed || problem !== null}
                      onClick={() => {
                        void save()
                      }}
                      className="rounded-full px-4 py-1.5 text-sm font-medium text-white shadow-sm disabled:opacity-50"
                      style={{ background: `linear-gradient(135deg, ${ACCENT.from}, ${ACCENT.to})` }}
                    >
                      {mine ? 'Update' : 'Save'}
                    </button>
                  </span>
                </div>
              </div>
            ) : (
              <p className="relative mt-4 rounded-xl bg-white/70 px-3 py-2 text-sm text-gray-500">You can read the reviews but not add your own.</p>
            )}

            {items.length > 0 && (
              <ul className="relative mt-4 space-y-2" aria-label="Reviews">
                {items.map((r) => {
                  const label = authorLabel(r)
                  const own = mine !== null && r.id === mine.id
                  return (
                    <li key={r.id} className="flex gap-3 rounded-2xl bg-white/70 p-3">
                      <Avatar authorId={r.authorId} label={label} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-x-2">
                          <span className="text-sm font-semibold text-gray-900">{label}</span>
                          {own && <span className="rounded-full bg-amber-100 px-2 text-xs text-amber-800">You</span>}
                          <StarRating rating={r.rating} scale={r.scale} halfStars size="sm" />
                          <span className="text-xs text-gray-500" title={new Date(r.createdAt).toLocaleString()}>
                            {relativeTime(now, r.createdAt)}
                          </span>
                          {isEdited(r) && (
                            <span className="text-xs text-gray-400" title={`Edited ${new Date(r.updatedAt).toLocaleString()}`}>
                              (edited)
                            </span>
                          )}
                          {r.scale !== scale && <span className="text-xs italic text-gray-400">given on a {r.scale}-star scale</span>}
                        </div>
                        {r.body !== '' && <p className="mt-1 whitespace-pre-wrap break-words text-sm text-gray-800">{r.body}</p>}
                        {r.canDelete && !own && (
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => {
                              void remove(r, false)
                            }}
                            className="mt-1 text-xs text-gray-500 underline hover:text-red-700 disabled:opacity-50"
                          >
                            Delete
                          </button>
                        )}
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}
            {nextBefore !== '' && (
              <div className="mt-3 text-center">
                <button
                  type="button"
                  disabled={loadingMore}
                  onClick={() => {
                    void loadMore()
                  }}
                  className="rounded-full border border-gray-300 bg-white px-4 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                >
                  {loadingMore ? 'Loading…' : 'Show older reviews'}
                </button>
              </div>
            )}
          </>
        )
      )}
      {dialog}
    </section>
  )
}
