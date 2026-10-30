import { useEffect, useMemo, useState } from 'react'
import { fetchMedia, deleteMedia, type MediaFile } from '../../../api/media'
import { fetchTools, type Tool } from '../../../api/tools'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { useConfirm } from '../../../Shared/Components/Modal/useConfirm'
import { useSnackBar } from '../../../Shared/Components/SnackBar/SnackBarContext'
import { Dropdown } from '../../../Shared/Components/Dropdown/Dropdown'
import { ImageUploadCropper } from '../../../Shared/Components/ImageUploadCropper/ImageUploadCropper'
import { MediaDetailModal } from './MediaDetailModal'
import { MediaTile } from './MediaTile'
import { applyMediaFilters, bulkDeleteMessage, defaultMediaFilters, sortOptions, typeOptions, type MediaFilters } from './mediaFilters'

// Reserved toolSlug for the core app itself (not an installed Tool) —
// see api/src/media/handler/upload_image_handler.go's own
// cinqoSystemToolSlug doc comment and
// plan/ai/media/step-08-admin-media-upload.md.
const CINQO_SYSTEM_TOOL_SLUG = 'cinqo'

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
  const [selectedFile, setSelectedFile] = useState<MediaFile | null>(null)
  const [filters, setFilters] = useState<MediaFilters>(defaultMediaFilters)
  const [checked, setChecked] = useState<Set<string>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
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
        // page's own grid (unfiltered) still works without it.
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
      setChecked((prev) => {
        const next = new Set(prev)
        next.delete(file.id)
        return next
      })
      successMessage('Media file deleted.')
    } catch (err) {
      errorMessage(err instanceof ApiError ? err.message : 'Failed to delete media file.')
    } finally {
      setBusyId(null)
    }
  }

  const visible = useMemo(() => (media ? applyMediaFilters(media, filters) : []), [media, filters])
  const checkedFiles = visible.filter((f) => checked.has(f.id))
  const allChecked = visible.length > 0 && checkedFiles.length === visible.length

  const toggle = (id: string) => {
    setChecked((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleBulkDelete = async () => {
    const targets = checkedFiles
    if (targets.length === 0) return
    const confirmed = await confirm({
      title: `Delete ${targets.length} media file${targets.length === 1 ? '' : 's'}`,
      message: bulkDeleteMessage(targets),
    })
    if (!confirmed) return
    setBulkBusy(true)
    const failed = new Set<string>()
    for (const file of targets) {
      try {
        await deleteMedia(file.id)
      } catch {
        failed.add(file.id)
      }
    }
    const deleted = new Set(targets.filter((f) => !failed.has(f.id)).map((f) => f.id))
    setMedia((prev) => (prev ? prev.filter((m) => !deleted.has(m.id)) : prev))
    setChecked(failed)
    setBulkBusy(false)
    if (failed.size === 0) successMessage(`Deleted ${deleted.size} media file${deleted.size === 1 ? '' : 's'}.`)
    else errorMessage(`Deleted ${deleted.size}, ${failed.size} failed.`)
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
    <div className="max-w-7xl space-y-6 p-6">
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
            sourceLabel={uploadTargetOptions.find((o) => o.value === uploadToolSlug)?.label ?? uploadToolSlug}
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

      {media.length > 0 && (
        <div className="flex flex-wrap items-end gap-3 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
          <label className="min-w-48 flex-1 text-xs font-medium text-gray-600">
            Name
            <input
              type="search"
              value={filters.query}
              onChange={(e) => {
                setFilters((f) => ({ ...f, query: e.target.value }))
              }}
              placeholder="Search by title or filename"
              className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-900"
            />
          </label>
          <div className="w-40">
            <Dropdown
              options={typeOptions}
              value={filters.type}
              onChange={(type) => {
                setFilters((f) => ({ ...f, type }))
              }}
            />
          </div>
          <div className="w-44">
            <Dropdown
              options={sortOptions}
              value={filters.sort}
              onChange={(sort) => {
                setFilters((f) => ({ ...f, sort }))
              }}
            />
          </div>
        </div>
      )}

      {media.length === 0 && <p className="py-6 text-center text-sm text-gray-400">No media files.</p>}
      {media.length > 0 && visible.length === 0 && (
        <p className="py-6 text-center text-sm text-gray-400">No media files match these filters.</p>
      )}

      {visible.length > 0 && (
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-4 rounded-lg bg-gray-100 px-4 py-2.5">
            <label className="flex cursor-pointer items-center gap-3 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={allChecked}
                onChange={() => {
                  setChecked(allChecked ? new Set() : new Set(visible.map((f) => f.id)))
                }}
                className="h-4 w-4 cursor-pointer accent-blue-600"
              />
              Select all media ({visible.length})
            </label>
            {checkedFiles.length > 0 && (
              <div className="flex items-center gap-3 text-sm">
                <span className="text-gray-600">{checkedFiles.length} selected</span>
                <button
                  type="button"
                  disabled={bulkBusy}
                  onClick={() => {
                    void handleBulkDelete()
                  }}
                  className="rounded-md border border-red-200 bg-white px-3 py-1 text-red-600 hover:bg-red-50 disabled:opacity-50"
                >
                  Delete selected
                </button>
              </div>
            )}
          </div>
          {/* A uniquely-named grid class, for the same reason as the card grids:
              an installed tool's globally-injected Tailwind stylesheet can
              redeclare a bare `.grid-cols-*` and collapse a plain Tailwind grid. */}
          <style>{`
            .media-tile-grid { display: grid; gap: 1.5rem 1rem; grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr)); }
          `}</style>
          <div className="media-tile-grid">
            {visible.map((file) => (
              <MediaTile
                key={file.id}
                file={file}
                selected={checked.has(file.id)}
                busy={busyId === file.id || bulkBusy}
                onToggle={() => {
                  toggle(file.id)
                }}
                onOpen={() => {
                  setSelectedFile(file)
                }}
                onDelete={() => {
                  void handleDelete(file)
                }}
              />
            ))}
          </div>
        </div>
      )}
      {dialog}
      {selectedFile && (
        <MediaDetailModal
          file={selectedFile}
          onClose={() => {
            setSelectedFile(null)
          }}
          onTitleSaved={(updated) => {
            setSelectedFile(updated)
            setMedia((prev) => (prev ? prev.map((m) => (m.id === updated.id ? updated : m)) : prev))
          }}
        />
      )}
    </div>
  )
}
