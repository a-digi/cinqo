import { useState } from 'react'
import { FullPageModal } from '../../../Shared/Components/FullPageModal/FullPageModal'
import { mediaDownloadUrl, updateMediaTitle, type MediaFile } from '../../../api/media'
import { ApiError } from '../../../api/client'
import { useSnackBar } from '../../../Shared/Components/SnackBar/SnackBarContext'

// formatSize duplicated from MediaListPage.tsx rather than shared —
// same "no existing shared helper for it" reasoning that file's own
// copy already used; two three-line copies aren't worth an
// abstraction.
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function MetadataRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-gray-400">{label}</dt>
      <dd className="mt-0.5 text-sm text-gray-900">{value}</dd>
    </div>
  )
}

// The combined preview+metadata modal (step 9) — one full-page overlay
// with the file's own preview on the left and its metadata (including
// the only editable field, Title) on the right. Opened by either
// clicking a table row or its Preview icon (MediaListPage.tsx); the
// left pane falls back to a plain placeholder for anything that isn't
// an image or a PDF, so this is also what a non-previewable row's row
// click opens — there is no separate "metadata-only" modal. See
// plan/ai/media/step-09-title-metadata-and-preview.md.
export interface MediaDetailModalProps {
  file: MediaFile
  onClose: () => void
  onTitleSaved: (updated: MediaFile) => void
}

export function MediaDetailModal({ file, onClose, onTitleSaved }: MediaDetailModalProps) {
  const [title, setTitle] = useState(file.title)
  const [saving, setSaving] = useState(false)
  const { successMessage, errorMessage } = useSnackBar()

  const trimmedTitle = title.trim()

  const handleSaveTitle = async () => {
    if (trimmedTitle === '' || trimmedTitle === file.title) return
    setSaving(true)
    try {
      const updated = await updateMediaTitle(file.id, trimmedTitle)
      onTitleSaved(updated)
      successMessage('Title saved.')
    } catch (err) {
      errorMessage(err instanceof ApiError ? err.message : 'Failed to save title.')
    } finally {
      setSaving(false)
    }
  }

  const isImage = file.contentType.startsWith('image/')
  const isPdf = file.contentType === 'application/pdf'

  return (
    <FullPageModal onClose={onClose}>
      <div className="flex min-h-0 flex-1">
        <div className="flex flex-1 items-center justify-center bg-gray-900 p-8">
          {isImage && (
            <img
              src={mediaDownloadUrl(file.id)}
              alt={file.title || file.originalFilename}
              className="max-h-full max-w-full object-contain"
            />
          )}
          {isPdf && (
            <iframe title={file.title || file.originalFilename} src={mediaDownloadUrl(file.id)} className="h-full w-full rounded-lg" />
          )}
          {!isImage && !isPdf && (
            <div className="flex flex-col items-center gap-2 text-gray-400">
              <div className="h-16 w-16 rounded-lg border-2 border-dashed border-gray-600" />
              <p className="text-sm">No preview available for this file type.</p>
            </div>
          )}
        </div>

        <div className="w-96 shrink-0 overflow-y-auto border-l border-gray-100 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-gray-400">Details</h2>

          <div className="mt-4">
            <label htmlFor="media-title" className="text-xs font-medium uppercase tracking-wide text-gray-400">
              Title
            </label>
            <div className="mt-1 flex gap-2">
              <input
                id="media-title"
                type="text"
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value)
                }}
                placeholder={file.originalFilename}
                className="w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-gray-900 focus:outline-none"
              />
              <button
                type="button"
                onClick={() => {
                  void handleSaveTitle()
                }}
                disabled={saving || trimmedTitle === '' || trimmedTitle === file.title}
                className="shrink-0 rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
              >
                {saving ? 'Saving…' : 'Save'}
              </button>
            </div>
          </div>

          <dl className="mt-6 space-y-4">
            <MetadataRow label="Filename" value={file.originalFilename} />
            <MetadataRow label="Tool" value={file.toolSlug} />
            <MetadataRow label="Content type" value={file.contentType} />
            <MetadataRow label="Size" value={formatSize(file.sizeBytes)} />
            {file.width !== null && file.height !== null && <MetadataRow label="Dimensions" value={`${file.width} × ${file.height}`} />}
            <MetadataRow label="Uploaded by" value={file.uploadedByUserId} />
            <MetadataRow label="Created" value={new Date(file.createdAt).toLocaleString()} />
            {file.updatedAt && <MetadataRow label="Updated" value={new Date(file.updatedAt).toLocaleString()} />}
            <MetadataRow label="Expires" value={file.expiresAt ? new Date(file.expiresAt).toLocaleString() : 'Never'} />
          </dl>
        </div>
      </div>
    </FullPageModal>
  )
}
