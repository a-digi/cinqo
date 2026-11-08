import { useRef, useState } from 'react'
import { ApiError } from '../../../api/client'
import { uploadImage, uploadMediaFile } from '../../../api/media'
import { Dropdown, type DropdownOption } from '../../../Shared/Components/Dropdown/Dropdown'
import { XIcon } from '../../../Shared/Components/IconButton/icons'
import { CropModal } from '../../../Shared/Components/ImageUploadCropper/CropModal'
import { formatSize } from './mediaFilters'
import { canCrop, canUpload, croppedFilename, toPendingUploads, usesImageEndpoint, type PendingUpload } from './uploadRules'

export interface MediaUploadModalProps {
  // Where the files can belong: the core app ("Cinqo") plus every installed tool.
  targets: DropdownOption[]
  defaultTarget: string
  onClose: () => void
  // Called once after a run in which at least one file was stored.
  onUploaded: (count: number) => void
}

// "+ Media": drop files (or browse), choose which tool or the system they
// belong to, give each a title, upload. Images the server can process go through
// the image endpoint (resized, JPEG); everything else is stored as-is. Files that
// fail stay in the list with their error so they can be retried.
export function MediaUploadModal({ targets, defaultTarget, onClose, onUploaded }: MediaUploadModalProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [target, setTarget] = useState(defaultTarget)
  const [items, setItems] = useState<PendingUpload[]>([])
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [cropSrc, setCropSrc] = useState<string | null>(null)

  const addFiles = (files: FileList | File[]) => {
    const added = toPendingUploads(Array.from(files))
    if (added.length > 0) setItems((prev) => [...prev, ...added])
  }

  const patch = (key: string, change: Partial<PendingUpload>) => {
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, ...change } : i)))
  }

  const closeCrop = () => {
    if (cropSrc) URL.revokeObjectURL(cropSrc)
    setCropSrc(null)
  }

  const handleCropped = (blob: Blob) => {
    const [item] = items
    patch(item.key, { file: new File([blob], croppedFilename(item.file.name), { type: 'image/jpeg' }) })
    closeCrop()
  }

  const handleUpload = async () => {
    setBusy(true)
    let stored = 0
    let failed = 0
    for (const item of items) {
      patch(item.key, { status: 'uploading', error: '' })
      try {
        const title = item.title.trim()
        if (usesImageEndpoint(item.file)) await uploadImage(item.file, target, title)
        else await uploadMediaFile(item.file, target, title)
        stored++
        setItems((prev) => prev.filter((i) => i.key !== item.key))
      } catch (err) {
        failed++
        patch(item.key, {
          status: 'error',
          error: err instanceof ApiError ? err.message : 'Failed to upload.',
        })
      }
    }
    setBusy(false)
    if (stored > 0) onUploaded(stored)
    if (failed === 0) onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-full w-full max-w-xl flex-col rounded-2xl bg-white shadow-2xl">
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4">
          <h2 className="text-sm font-semibold text-gray-900">Add media</h2>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            aria-label="Close"
            className="rounded-full p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50"
          >
            <XIcon />
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
          <div>
            <span className="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500">Belongs to</span>
            <Dropdown options={targets} value={target} onChange={setTarget} disabled={busy} />
          </div>

          <input
            ref={inputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              if (e.target.files) addFiles(e.target.files)
              e.target.value = '' // allow picking the same file again
            }}
          />
          <button
            type="button"
            disabled={busy}
            onClick={() => inputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault()
              setDragging(true)
            }}
            onDragLeave={() => {
              setDragging(false)
            }}
            onDrop={(e) => {
              e.preventDefault()
              setDragging(false)
              addFiles(e.dataTransfer.files)
            }}
            className={`flex w-full flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed px-4 py-10 text-center transition-colors disabled:opacity-50 ${
              dragging ? 'border-blue-500 bg-blue-50' : 'border-gray-300 bg-gray-50 hover:border-gray-400'
            }`}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className="h-8 w-8 text-gray-400">
              <path d="M12 16V5m0 0-4 4m4-4 4 4M5 19h14" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            <span className="text-sm font-medium text-gray-700">Drag and drop files here</span>
            <span className="text-xs text-gray-500">or click to browse</span>
          </button>

          {items.length > 0 && (
            <ul className="space-y-2">
              {items.map((item) => (
                <li key={item.key} className="rounded-lg border border-gray-200 p-3">
                  <div className="flex items-center justify-between gap-2 text-xs text-gray-500">
                    <span className="truncate" title={item.file.name}>
                      {item.file.name} · {formatSize(item.file.size)}
                    </span>
                    <div className="flex shrink-0 items-center gap-2">
                      {canCrop(items) && (
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => {
                            setCropSrc(URL.createObjectURL(item.file))
                          }}
                          className="rounded border border-gray-200 px-2 py-0.5 font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
                        >
                          Crop
                        </button>
                      )}
                      <button
                        type="button"
                        disabled={busy}
                        aria-label={`Remove ${item.file.name}`}
                        onClick={() => {
                          setItems((prev) => prev.filter((i) => i.key !== item.key))
                        }}
                        className="shrink-0 rounded-full p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50"
                      >
                        <XIcon />
                      </button>
                    </div>
                  </div>
                  <input
                    type="text"
                    value={item.title}
                    disabled={busy}
                    onChange={(e) => {
                      patch(item.key, { title: e.target.value })
                    }}
                    aria-label={`Title for ${item.file.name}`}
                    placeholder="Title (required)"
                    className="mt-2 w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-gray-900 focus:outline-none"
                  />
                  {item.status === 'uploading' && <p className="mt-1 text-xs text-gray-500">Uploading…</p>}
                  {item.status === 'error' && <p className="mt-1 text-xs text-red-600">{item.error}</p>}
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex justify-end gap-2 rounded-b-2xl border-t border-gray-100 bg-gray-50 px-5 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="rounded-md border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              void handleUpload()
            }}
            disabled={busy || !canUpload(items)}
            className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {busy ? 'Uploading…' : items.length > 1 ? `Upload ${items.length} files` : 'Upload'}
          </button>
        </div>
      </div>
      {cropSrc && (
        <CropModal
          imageSrc={cropSrc}
          sourceLabel={targets.find((t) => t.value === target)?.label ?? target}
          requireTitle={false}
          onCancel={closeCrop}
          onSave={handleCropped}
        />
      )}
    </div>
  )
}
