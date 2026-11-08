import { apiGet, apiDelete } from './client'

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
