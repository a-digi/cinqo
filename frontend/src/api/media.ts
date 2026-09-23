import { apiGet, apiDelete, apiUpload, apiPatch } from './client'

// Same snake_case-to-camelCase *Raw mapping convention as api/tools.ts,
// api/auth.ts. storedPath is never sent by the backend (deliberately
// omitted from mediaFileResponse — see
// api/src/media/handler/list_handler.go's own doc comment) — nothing
// server-internal to map here.
export interface MediaFile {
  id: string
  toolSlug: string
  originalFilename: string
  extension: string
  contentType: string
  sizeBytes: number
  uploadedByUserId: string
  // '' when the row has none recorded (e.g. Career's own CV imports,
  // whose upload happens before the AI conversation exists) — kept as
  // a plain string, not null, matching every other *Raw mapping in
  // this file's own convention for an absent optional field.
  conversationId: string
  expiresAt: string
  createdAt: string
  // null when the row isn't an image (see MediaFile entity's own doc
  // comment — width/height are only ever set by the image endpoints).
  width: number | null
  height: number | null
  // '' when never replaced, matching this file's own existing
  // convention for an absent optional field.
  updatedAt: string
  // '' when not set — plain, non-localized, admin-editable label. See
  // plan/ai/media/step-09-title-metadata-and-preview.md.
  title: string
}

interface MediaFileRaw {
  id: string
  tool_slug: string
  original_filename: string
  extension: string
  content_type: string
  size_bytes: number
  uploaded_by_user_id: string
  conversation_id?: string
  expires_at?: string
  created_at: string
  width?: number
  height?: number
  updated_at?: string
  title?: string
}

function fromRaw(raw: MediaFileRaw): MediaFile {
  return {
    id: raw.id,
    toolSlug: raw.tool_slug,
    originalFilename: raw.original_filename,
    extension: raw.extension,
    contentType: raw.content_type,
    sizeBytes: raw.size_bytes,
    uploadedByUserId: raw.uploaded_by_user_id,
    conversationId: raw.conversation_id ?? '',
    expiresAt: raw.expires_at ?? '',
    createdAt: raw.created_at,
    width: raw.width ?? null,
    height: raw.height ?? null,
    updatedAt: raw.updated_at ?? '',
    title: raw.title ?? '',
  }
}

// toolSlug omitted or '' lists every tool's own media.
export async function fetchMedia(toolSlug?: string): Promise<MediaFile[]> {
  const query = toolSlug ? `?toolSlug=${encodeURIComponent(toolSlug)}` : ''
  const raw = await apiGet<{ message: MediaFileRaw[] }>(`/api/v1/media${query}`)
  return raw.message.map(fromRaw)
}

export async function deleteMedia(id: string): Promise<void> {
  await apiDelete(`/api/v1/media/${encodeURIComponent(id)}`)
}

// Empty string clears the title back to "not set" — see
// api/src/media/handler/update_title_handler.go's own doc comment.
export async function updateMediaTitle(id: string, title: string): Promise<MediaFile> {
  const raw = await apiPatch<{ message: MediaFileRaw }>(`/api/v1/media/${encodeURIComponent(id)}`, { title })
  return fromRaw(raw.message)
}

// toolSlug omitted or '' lists every tool's own media the CALLER
// uploaded — ownership is enforced server-side (GET /api/v1/media/mine),
// never by this filter.
export async function fetchMyMedia(toolSlug?: string): Promise<MediaFile[]> {
  const query = toolSlug ? `?toolSlug=${encodeURIComponent(toolSlug)}` : ''
  const raw = await apiGet<{ message: MediaFileRaw[] }>(`/api/v1/media/mine${query}`)
  return raw.message.map(fromRaw)
}

export function mediaDownloadUrl(id: string): string {
  return `/api/v1/media/${encodeURIComponent(id)}/download`
}

// Public, unauthenticated URL — safe to embed in an <img src>, share
// with a logged-out visitor, or fetch from a Tool's own backend with no
// browser session at all. Only ever serves rows that are actual images
// (see api/src/media/handler/public_image_handler.go's own doc
// comment) — use mediaDownloadUrl instead for anything else, or when
// ownership-checked access is what's actually wanted.
export function publicImageUrl(id: string): string {
  return `/api/v1/media/images/${encodeURIComponent(id)}/public`
}

interface ImageUploadResult {
  fileId: string
  width: number
  height: number
}

// Creates a new image row — server resizes/re-encodes (see
// api/src/media/handler/upload_image_handler.go's own doc comment).
export async function uploadImage(file: Blob, toolSlug: string): Promise<ImageUploadResult> {
  const formData = new FormData()
  formData.append('file', file, 'image.jpg')
  formData.append('toolSlug', toolSlug)
  const raw = await apiUpload<{ message: ImageUploadResult }>('/api/v1/media/images', formData)
  return raw.message
}

// Replaces an EXISTING image row's own bytes in place, keeping id
// stable (step 1's own "overwrite the old one" capability).
export async function replaceImage(id: string, file: Blob): Promise<ImageUploadResult> {
  const formData = new FormData()
  formData.append('file', file, 'image.jpg')
  const raw = await apiUpload<{ message: ImageUploadResult }>(`/api/v1/media/images/${encodeURIComponent(id)}`, formData, 'PUT')
  return raw.message
}
