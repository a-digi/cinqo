import { apiDelete, apiGet, apiPatch, apiPost } from './client'

// The comments routes answer in camelCase already (api/src/comments/handler), so
// there is no *Raw mapping layer.
export interface Comment {
  id: string
  entityType: string
  entityId: string
  body: string
  authorId: string
  // '' when the identity provider gave no name; the screen shows "User".
  authorName: string
  createdAt: string
  // '' until the comment is edited.
  updatedAt: string
  // What the signed-in user may do with this comment, decided by the server.
  canEdit: boolean
  canDelete: boolean
  // The comment this one replies to; '' for a top-level comment.
  parentId: string
  // 0 for a top-level comment, its parent's depth + 1 for a reply.
  depth: number
  // True only for a comment shown just because a reply below it matched the filters.
  context: boolean
}

export interface CommentPage {
  comments: Comment[]
  // Of the comments matching the filters, not just this page.
  total: number
  // The id to pass as `before` for the next (older) page; '' when there is none.
  nextBefore: string
  // Whether the signed-in user may add a comment here (false for a read-only user).
  canCreate: boolean
}

export interface CommentsTarget {
  owner: string
  entityType: string
  entityId: string
}

export interface CommentFilters {
  limit?: number
  before?: string
  authorId?: string
  createdFrom?: string
  createdTo?: string
}

function base(t: CommentsTarget): string {
  return `/api/v1/comments/${encodeURIComponent(t.owner)}/${encodeURIComponent(t.entityType)}/${encodeURIComponent(t.entityId)}`
}

export async function fetchComments(t: CommentsTarget, filters: CommentFilters = {}): Promise<CommentPage> {
  const q = new URLSearchParams()
  if (filters.limit !== undefined) q.set('limit', String(filters.limit))
  if (filters.before) q.set('before', filters.before)
  if (filters.authorId) q.set('authorId', filters.authorId)
  if (filters.createdFrom) q.set('createdFrom', filters.createdFrom)
  if (filters.createdTo) q.set('createdTo', filters.createdTo)
  const query = q.toString()
  const raw = await apiGet<{ message: CommentPage }>(`${base(t)}${query ? `?${query}` : ''}`)
  return raw.message
}

// parentId makes the new comment a reply to that comment; leave it out for a top-level one.
export async function createComment(t: CommentsTarget, body: string, parentId?: string): Promise<Comment> {
  const raw = await apiPost<{ message: Comment }>(base(t), parentId ? { body, parentId } : { body })
  return raw.message
}

export async function editComment(t: CommentsTarget, id: string, body: string): Promise<Comment> {
  const raw = await apiPatch<{ message: Comment }>(`${base(t)}/${encodeURIComponent(id)}`, { body })
  return raw.message
}

export async function deleteComment(t: CommentsTarget, id: string): Promise<void> {
  await apiDelete(`${base(t)}/${encodeURIComponent(id)}`)
}
