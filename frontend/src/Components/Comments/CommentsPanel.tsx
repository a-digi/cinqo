import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../../api/client'
import { fetchAuthMe } from '../../api/auth'
import { createComment, deleteComment, editComment, fetchComments, type Comment, type CommentsTarget } from '../../api/comments'
import { useConfirm } from '../../Shared/Components/Modal/useConfirm'
import { IconBadge } from '../../Shared/Components/CardArt/icons'
import {
  MAX_BODY,
  NO_FILTERS,
  PAGE_SIZE,
  PANEL_ACCENT,
  accentForAuthor,
  authorLabel,
  bodyProblem,
  canReplyTo,
  charsUsed,
  deleteMessage,
  filterParams,
  hasFilters,
  indentLevel,
  initialOf,
  insertReply,
  isEdited,
  mergeOlder,
  rangeProblem,
  relativeTime,
  removeThread,
  replyCount,
  type FilterState,
} from './commentRules'

function errorText(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

function gradient(a: { from: string; to: string }): string {
  return `linear-gradient(135deg, ${a.from}, ${a.to})`
}

const inputClass = 'rounded-xl border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-violet-500 focus:outline-none'

function Avatar({ authorId, label }: { authorId: string; label: string }) {
  const accent = accentForAuthor(authorId)
  return (
    <span
      className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
      style={{ background: gradient(accent) }}
      aria-hidden="true"
    >
      {initialOf(label)}
    </span>
  )
}

interface CommentRowProps {
  comment: Comment
  now: Date
  busy: boolean
  // The accent of the author this is a reply to (for the guide line); undefined for a top-level comment.
  guide?: string
  // Whether the signed-in user may reply to this comment here.
  canReply: boolean
  onEdit: (id: string, body: string) => Promise<boolean>
  onReply: (parent: Comment, body: string) => Promise<boolean>
  onDelete: (comment: Comment) => void
}

function CommentRow({ comment, now, busy, guide, canReply, onEdit, onReply, onDelete }: CommentRowProps) {
  const [editing, setEditing] = useState(false)
  const [replying, setReplying] = useState(false)
  const [replyDraft, setReplyDraft] = useState('')
  const [draft, setDraft] = useState(comment.body)
  const replyProblem = bodyProblem(replyDraft)
  const label = authorLabel(comment)
  const problem = bodyProblem(draft)

  async function save() {
    if (problem) return
    if (await onEdit(comment.id, draft)) setEditing(false)
  }

  async function sendReply() {
    if (replyProblem) return
    if (await onReply(comment, replyDraft)) {
      setReplying(false)
      setReplyDraft('')
    }
  }

  const level = indentLevel(comment.depth)
  const showActions = !editing && !comment.context && (canReply || comment.canEdit || comment.canDelete)

  return (
    <li
      className={`flex gap-3 rounded-2xl bg-white/70 p-3 ${comment.context ? 'opacity-60' : ''}`}
      // Each level steps in by --comment-indent (narrower on a phone, set on the list).
      style={{
        marginLeft: `calc(${level} * var(--comment-indent, 20px))`,
        ...(guide ? { borderLeft: `3px solid ${guide}` } : undefined),
      }}
    >
      <Avatar authorId={comment.authorId} label={label} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-sm font-semibold text-gray-900">{label}</span>
          <span className="text-xs text-gray-500" title={new Date(comment.createdAt).toLocaleString()}>
            {relativeTime(now, comment.createdAt)}
          </span>
          {comment.context && <span className="text-xs italic text-gray-400">shown for context</span>}
          {isEdited(comment) && (
            <span className="text-xs text-gray-400" title={`Edited ${new Date(comment.updatedAt).toLocaleString()}`}>
              (edited)
            </span>
          )}
        </div>

        {editing ? (
          <div className="mt-1 space-y-2">
            <textarea
              value={draft}
              rows={3}
              onChange={(e) => {
                setDraft(e.target.value)
              }}
              aria-label="Edit comment"
              className={`w-full ${inputClass}`}
            />
            <div className="flex items-center gap-2">
              <button
                type="button"
                disabled={busy || problem !== null}
                onClick={() => {
                  void save()
                }}
                className="rounded-full px-3 py-1 text-xs font-medium text-white disabled:opacity-50"
                style={{ background: gradient(PANEL_ACCENT) }}
              >
                Save
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setEditing(false)
                  setDraft(comment.body)
                }}
                className="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Cancel
              </button>
              {problem && draft.trim() !== '' && <span className="text-xs text-red-600">{problem}</span>}
            </div>
          </div>
        ) : (
          <p className="mt-0.5 whitespace-pre-wrap break-words text-sm text-gray-800">{comment.body}</p>
        )}

        {showActions && (
          <div className="mt-1 flex gap-3 text-xs">
            {canReply && (
              <button
                type="button"
                disabled={busy}
                aria-expanded={replying}
                onClick={() => {
                  setReplying((v) => !v)
                }}
                className="text-gray-500 hover:text-gray-900 disabled:opacity-50"
              >
                Reply
              </button>
            )}
            {comment.canEdit && (
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setDraft(comment.body)
                  setEditing(true)
                }}
                className="text-gray-500 hover:text-gray-900 disabled:opacity-50"
              >
                Edit
              </button>
            )}
            {comment.canDelete && (
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  onDelete(comment)
                }}
                className="text-gray-500 hover:text-red-600 disabled:opacity-50"
              >
                Delete
              </button>
            )}
          </div>
        )}

        {replying && (
          <div className="mt-2 space-y-2">
            <textarea
              // eslint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              value={replyDraft}
              rows={2}
              placeholder={`Reply to ${label}…`}
              aria-label={`Reply to ${label}`}
              onChange={(e) => {
                setReplyDraft(e.target.value)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  void sendReply()
                }
              }}
              className={`w-full ${inputClass}`}
            />
            <div className="flex items-center gap-2">
              <button
                type="button"
                disabled={busy || replyProblem !== null}
                onClick={() => {
                  void sendReply()
                }}
                className="rounded-full px-3 py-1 text-xs font-medium text-white disabled:opacity-50"
                style={{ background: gradient(PANEL_ACCENT) }}
              >
                Reply
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setReplying(false)
                  setReplyDraft('')
                }}
                className="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Cancel
              </button>
              {replyProblem && replyDraft.trim() !== '' && <span className="text-xs text-red-600">{replyProblem}</span>}
            </div>
          </div>
        )}
      </div>
    </li>
  )
}

export interface CommentsPanelProps extends CommentsTarget {
  title?: string
}

// The comments for one entity: add, reply, edit and delete your own, filter by author and
// date, load older ones. Mounted into a tool's page by bridge.mountComments (see
// mountComments.tsx). Everything is plain text; the server decides what the signed-in
// user may do and says so with canEdit / canDelete. A reply is drawn directly below its
// parent, stepped in by its depth. See plan/ai/comments/comments-engine.md and
// comment-replies.md.
export function CommentsPanel({ owner, entityType, entityId, title = 'Comments' }: CommentsPanelProps) {
  const target: CommentsTarget = { owner, entityType, entityId }
  const [items, setItems] = useState<Comment[]>([])
  const [total, setTotal] = useState(0)
  const [nextBefore, setNextBefore] = useState('')
  // True until a load says otherwise; the server decides (a reader may not add comments).
  const [canCreate, setCanCreate] = useState(true)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState('')
  const [filters, setFilters] = useState<FilterState>(NO_FILTERS)
  const [showFilters, setShowFilters] = useState(false)
  const [userId, setUserId] = useState('')
  const [now, setNow] = useState(() => new Date())
  const { confirm, dialog } = useConfirm()
  // Only the newest load may write to the screen (a slower, older answer is dropped).
  const loadSeq = useRef(0)

  useEffect(() => {
    let cancelled = false
    fetchAuthMe()
      .then((me) => {
        if (!cancelled) setUserId(me.userId)
      })
      .catch(() => undefined) // "Mine" simply stays unavailable
    return () => {
      cancelled = true
    }
  }, [])

  // Keep "5m ago" honest while the page stays open.
  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(new Date())
    }, 60_000)
    return () => {
      window.clearInterval(timer)
    }
  }, [])

  const rangeError = rangeProblem(filters)
  const params = filterParams(filters, userId)
  const paramsKey = JSON.stringify(params)

  const load = useCallback(async () => {
    const seq = ++loadSeq.current
    setLoading(true)
    setError('')
    try {
      const page = await fetchComments({ owner, entityType, entityId }, { limit: PAGE_SIZE, ...(JSON.parse(paramsKey) as object) })
      if (seq !== loadSeq.current) return
      setItems(page.comments)
      setTotal(page.total)
      setNextBefore(page.nextBefore)
      setCanCreate(page.canCreate)
    } catch (err) {
      if (seq !== loadSeq.current) return
      setItems([])
      setTotal(0)
      setNextBefore('')
      setError(errorText(err, "Couldn't load the comments."))
    } finally {
      if (seq === loadSeq.current) setLoading(false)
    }
  }, [owner, entityType, entityId, paramsKey])

  useEffect(() => {
    if (rangeError) return
    void load()
  }, [load, rangeError])

  async function loadMore() {
    setLoadingMore(true)
    setError('')
    try {
      const page = await fetchComments(target, { limit: PAGE_SIZE, before: nextBefore, ...params })
      setItems((prev) => mergeOlder(prev, page.comments))
      setTotal(page.total)
      setNextBefore(page.nextBefore)
    } catch (err) {
      setError(errorText(err, "Couldn't load more comments."))
    } finally {
      setLoadingMore(false)
    }
  }

  async function add() {
    if (bodyProblem(draft) !== null) return
    setBusy(true)
    setError('')
    try {
      const created = await createComment(target, draft)
      setDraft('')
      // A new comment belongs at the top, unless a filter would hide it.
      if (!hasFilters(filters)) {
        setItems((prev) => [created, ...prev])
        setTotal((t) => t + 1)
      } else {
        void load()
      }
    } catch (err) {
      setError(errorText(err, "Couldn't add the comment."))
    } finally {
      setBusy(false)
    }
  }

  async function reply(parent: Comment, body: string): Promise<boolean> {
    setBusy(true)
    setError('')
    try {
      const created = await createComment(target, body, parent.id)
      if (!hasFilters(filters)) {
        setItems((prev) => insertReply(prev, created))
        setTotal((t) => t + 1)
      } else {
        void load()
      }
      return true
    } catch (err) {
      setError(errorText(err, "Couldn't add the reply."))
      return false
    } finally {
      setBusy(false)
    }
  }

  async function edit(id: string, body: string): Promise<boolean> {
    setBusy(true)
    setError('')
    try {
      const updated = await editComment(target, id, body)
      setItems((prev) => prev.map((c) => (c.id === id ? updated : c)))
      return true
    } catch (err) {
      setError(errorText(err, "Couldn't save the change."))
      return false
    } finally {
      setBusy(false)
    }
  }

  async function remove(comment: Comment) {
    const ok = await confirm({
      title: 'Delete comment',
      message: deleteMessage(replyCount(items, comment.id), hasFilters(filters)),
      confirmLabel: 'Delete',
    })
    if (!ok) return
    setBusy(true)
    setError('')
    try {
      await deleteComment(target, comment.id)
      if (hasFilters(filters)) {
        // Only some of the replies are shown, so the count is not known: ask again.
        void load()
      } else {
        const gone = replyCount(items, comment.id) + 1
        setItems((prev) => removeThread(prev, comment.id))
        setTotal((t) => Math.max(0, t - gone))
      }
    } catch (err) {
      setError(errorText(err, "Couldn't delete the comment."))
    } finally {
      setBusy(false)
    }
  }

  // Who wrote each shown comment, so a reply's guide line takes its parent author's colour.
  const authorOf = new Map(items.map((c) => [c.id, c.authorId]))
  const draftProblem = bodyProblem(draft)
  const used = charsUsed(draft)
  const filtered = hasFilters(filters)

  return (
    <section
      className="relative overflow-hidden rounded-3xl border p-5 shadow-sm"
      style={{ borderColor: PANEL_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 40%, ${PANEL_ACCENT.soft} 100%)` }}
      aria-label={title}
    >
      <div
        className="pointer-events-none absolute -right-8 -top-8 h-28 w-28 rounded-full opacity-15"
        style={{ background: gradient(PANEL_ACCENT) }}
      />

      <div className="relative flex items-center gap-3">
        <IconBadge name="chat" from={PANEL_ACCENT.from} to={PANEL_ACCENT.to} />
        <h2 className="min-w-0 flex-1 truncate text-base font-semibold text-gray-900">{title}</h2>
        <span
          className="rounded-full px-2.5 py-0.5 text-xs font-semibold"
          style={{ background: PANEL_ACCENT.soft, color: PANEL_ACCENT.text }}
        >
          {total}
        </span>
        <button
          type="button"
          aria-expanded={showFilters}
          onClick={() => {
            setShowFilters((v) => !v)
          }}
          className="rounded-full border border-gray-300 bg-white px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
        >
          Filters{filtered ? ' •' : ''}
        </button>
      </div>

      {showFilters && (
        <div className="relative mt-3 flex flex-wrap items-end gap-3 rounded-2xl bg-white/70 p-3">
          <div className="inline-flex overflow-hidden rounded-full border border-gray-300 text-xs">
            {[
              { value: false, label: 'Everyone' },
              { value: true, label: 'Only mine' },
            ].map((o) => (
              <button
                key={o.label}
                type="button"
                disabled={o.value && userId === ''}
                aria-pressed={filters.mine === o.value}
                onClick={() => {
                  setFilters((f) => ({ ...f, mine: o.value }))
                }}
                className={`px-3 py-1 disabled:opacity-40 ${filters.mine === o.value ? 'text-white' : 'bg-white text-gray-700 hover:bg-gray-50'}`}
                style={filters.mine === o.value ? { background: gradient(PANEL_ACCENT) } : undefined}
              >
                {o.label}
              </button>
            ))}
          </div>
          <label className="text-xs text-gray-500">
            From
            <input
              type="date"
              value={filters.from}
              onChange={(e) => {
                setFilters((f) => ({ ...f, from: e.target.value }))
              }}
              className={`mt-0.5 block ${inputClass} py-1`}
            />
          </label>
          <label className="text-xs text-gray-500">
            To
            <input
              type="date"
              value={filters.to}
              onChange={(e) => {
                setFilters((f) => ({ ...f, to: e.target.value }))
              }}
              className={`mt-0.5 block ${inputClass} py-1`}
            />
          </label>
          {filtered && (
            <button
              type="button"
              onClick={() => {
                setFilters(NO_FILTERS)
              }}
              className="text-xs text-gray-500 underline hover:text-gray-900"
            >
              Clear
            </button>
          )}
          {rangeError && <p className="w-full text-xs text-red-600">{rangeError}</p>}
        </div>
      )}

      {canCreate ? (
        <div className="relative mt-4 space-y-2">
          <textarea
            value={draft}
            rows={3}
            placeholder="Add a comment…"
            aria-label="New comment"
            onChange={(e) => {
              setDraft(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault()
                void add()
              }
            }}
            className={`w-full ${inputClass}`}
          />
          <div className="flex items-center justify-between gap-3">
            <span className={`text-xs ${used > MAX_BODY ? 'text-red-600' : 'text-gray-400'}`}>
              {used.toLocaleString('en')} / {MAX_BODY.toLocaleString('en')} · Ctrl/⌘+Enter to send
            </span>
            <button
              type="button"
              disabled={busy || draftProblem !== null}
              onClick={() => {
                void add()
              }}
              className="rounded-full px-4 py-1.5 text-sm font-medium text-white shadow-sm disabled:opacity-50"
              style={{ background: gradient(PANEL_ACCENT) }}
            >
              Comment
            </button>
          </div>
        </div>
      ) : (
        <p className="relative mt-4 rounded-xl bg-white/70 px-3 py-2 text-sm text-gray-500">
          You can read the comments but not add new ones.
        </p>
      )}

      {error && (
        <p role="alert" className="relative mt-3 rounded-xl bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      )}

      <div className="relative mt-4">
        {loading ? (
          <div className="animate-pulse space-y-2" aria-label="Loading comments">
            <div className="h-14 rounded-2xl bg-white/70" />
            <div className="h-14 rounded-2xl bg-white/70" />
          </div>
        ) : items.length === 0 ? (
          !error && (
            <p className="py-3 text-center text-sm text-gray-400">
              {filtered ? 'No comments match these filters.' : 'No comments yet. Be the first.'}
            </p>
          )
        ) : (
          <ul className="space-y-2 [--comment-indent:12px] sm:[--comment-indent:20px]">
            {items.map((c) => (
              <CommentRow
                key={c.id}
                comment={c}
                now={now}
                busy={busy}
                guide={c.parentId ? accentForAuthor(authorOf.get(c.parentId) ?? '').to : undefined}
                canReply={canCreate && canReplyTo(c)}
                onEdit={edit}
                onReply={reply}
                onDelete={(comment) => {
                  void remove(comment)
                }}
              />
            ))}
          </ul>
        )}
        {nextBefore !== '' && !loading && (
          <div className="mt-3 text-center">
            <button
              type="button"
              disabled={loadingMore}
              onClick={() => {
                void loadMore()
              }}
              className="rounded-full border border-gray-300 bg-white px-4 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              {loadingMore ? 'Loading…' : 'Show older comments'}
            </button>
          </div>
        )}
      </div>
      {dialog}
    </section>
  )
}
