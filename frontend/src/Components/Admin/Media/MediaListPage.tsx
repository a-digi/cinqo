import { useEffect, useState } from 'react'
import { fetchMedia, deleteMedia, mediaDownloadUrl, type MediaFile } from '../../../api/media'
import { fetchTools, type Tool } from '../../../api/tools'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { useSnackBar } from '../../../Shared/Components/SnackBar/SnackBarContext'
import { IconButton } from '../../../Shared/Components/IconButton/IconButton'
import { TrashIcon } from '../../../Shared/Components/IconButton/icons'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { Dropdown } from '../../../Shared/Components/Dropdown/Dropdown'
import { ImageUploadCropper } from '../../../Shared/Components/ImageUploadCropper/ImageUploadCropper'

// Reserved toolSlug for the core app itself (not an installed Tool) —
// see api/src/media/handler/upload_image_handler.go's own
// cinqoSystemToolSlug doc comment and
// plan/ai/media/step-08-admin-media-upload.md.
const CINQO_SYSTEM_TOOL_SLUG = 'cinqo'

// formatSize is local to this page — no existing shared helper for it
// (checked: no other admin page needs a byte count formatted).
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// Admin-only, read/audit/cleanup view of the core Media feature
// (api/src/media) — every tool's own uploaded files in one place,
// namespaced by tool. Non-image files still only ever arrive here from
// within a tool's own flow (e.g. Career's CV import); images can also
// be uploaded/replaced directly from this page (step 8 — folded in from
// the now-removed standalone "My Images" page), tagged by which
// tool_slug bucket they belong to, including the reserved "Cinqo"
// bucket for system-level images not owned by any installed Tool. See
// plan/ai/media/step-08-admin-media-upload.md.
export function MediaListPage() {
  const [tools, setTools] = useState<Tool[] | null>(null)
  const [media, setMedia] = useState<MediaFile[] | null>(null)
  const [toolSlug, setToolSlug] = useState('')
  const [uploadToolSlug, setUploadToolSlug] = useState(CINQO_SYSTEM_TOOL_SLUG)
  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const { confirm, dialog } = useConfirm()
  const { successMessage, errorMessage } = useSnackBar()

  const load = (slug: string) => {
    setError(null)
    fetchMedia(slug || undefined)
      .then(setMedia)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Failed to load media.')
      })
  }

  useEffect(() => {
    fetchTools()
      .then(setTools)
      .catch(() => {
        // Non-fatal: the tool filter dropdown just stays empty — the
        // page's own table (unfiltered) still works without it.
        setTools([])
      })
  }, [])

  useEffect(() => {
    load(toolSlug)
  }, [toolSlug])

  const handleDelete = async (file: MediaFile) => {
    const confirmed = await confirm({
      title: 'Delete media file',
      message: `Delete "${file.originalFilename}"? This removes the stored file and cannot be undone.`,
    })
    if (!confirmed) return
    setBusyId(file.id)
    try {
      await deleteMedia(file.id)
      setMedia((prev) => (prev ? prev.filter((m) => m.id !== file.id) : prev))
      successMessage('Media file deleted.')
    } catch (err) {
      errorMessage(err instanceof ApiError ? err.message : 'Failed to delete media file.')
    } finally {
      setBusyId(null)
    }
  }

  if (error) {
    return <p className="p-6 text-sm text-red-600">{error}</p>
  }

  if (!media || !tools) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading media…" />
      </div>
    )
  }

  const realToolOptions = tools.map((t) => ({ value: t.slug, label: t.name }))
  const toolOptions = [{ value: '', label: 'All tools' }, { value: CINQO_SYSTEM_TOOL_SLUG, label: 'Cinqo' }, ...realToolOptions]
  const uploadTargetOptions = [{ value: CINQO_SYSTEM_TOOL_SLUG, label: 'Cinqo' }, ...realToolOptions]

  return (
    <div className="max-w-5xl space-y-6 p-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Media</h1>
        <p className="mt-1 text-sm text-gray-500">
          Files uploaded by tools (or Cinqo itself) through the core Media feature — audit, upload, and clean up storage here.
        </p>
      </div>

      <div className="flex items-end justify-between gap-4">
        <div className="flex items-end gap-2">
          <div className="w-40">
            <Dropdown options={uploadTargetOptions} value={uploadToolSlug} onChange={setUploadToolSlug} />
          </div>
          <ImageUploadCropper
            toolSlug={uploadToolSlug}
            onDone={() => {
              successMessage('Image uploaded.')
              load(toolSlug)
            }}
            onError={(msg) => {
              errorMessage(msg)
            }}
          />
        </div>

        {tools.length > 0 && (
          <div className="w-64">
            <Dropdown options={toolOptions} value={toolSlug} onChange={setToolSlug} placeholder="All tools" />
          </div>
        )}
      </div>

      {media.length === 0 && <p className="py-6 text-center text-sm text-gray-400">No media files.</p>}

      {media.length > 0 && (
        <div className="rounded-lg border border-gray-200">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-4 py-2 font-medium" />
                <th className="px-4 py-2 font-medium">Filename</th>
                <th className="px-4 py-2 font-medium">Tool</th>
                <th className="px-4 py-2 font-medium">Size</th>
                <th className="px-4 py-2 font-medium">Uploaded</th>
                <th className="px-4 py-2 font-medium">Expires</th>
                <th className="px-4 py-2 font-medium" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {media.map((file) => (
                <tr key={file.id}>
                  <td className="px-4 py-2">
                    {file.width !== null ? (
                      <img src={mediaDownloadUrl(file.id)} alt="" className="h-10 w-10 rounded object-cover" />
                    ) : (
                      <div className="h-10 w-10 rounded bg-gray-100" />
                    )}
                  </td>
                  <td className="px-4 py-2 text-gray-900">{file.originalFilename}</td>
                  <td className="px-4 py-2">
                    <Pill outline>{file.toolSlug}</Pill>
                  </td>
                  <td className="px-4 py-2 text-gray-600">{formatSize(file.sizeBytes)}</td>
                  <td className="px-4 py-2 text-gray-600">{new Date(file.createdAt).toLocaleString()}</td>
                  <td className="px-4 py-2 text-gray-600">{file.expiresAt ? new Date(file.expiresAt).toLocaleString() : 'Never'}</td>
                  <td className="px-4 py-2">
                    <div className="flex items-center justify-end gap-2">
                      {file.width !== null && (
                        <ImageUploadCropper
                          toolSlug={file.toolSlug}
                          existingFileId={file.id}
                          onDone={() => {
                            successMessage('Image replaced.')
                            load(toolSlug)
                          }}
                          onError={(msg) => {
                            errorMessage(msg)
                          }}
                        />
                      )}
                      <IconButton
                        icon={<TrashIcon />}
                        label="Delete"
                        variant="danger"
                        onClick={() => {
                          void handleDelete(file)
                        }}
                        disabled={busyId === file.id}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {dialog}
    </div>
  )
}
