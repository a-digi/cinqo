import { useRef, useState } from 'react'
import { CropModal } from './CropModal'
import { uploadImage, replaceImage } from '../../../api/media'
import { ApiError } from '../../../api/client'

// Controlled, reusable "pick an image -> crop it -> upload/replace"
// component with no knowledge of WHERE it's used — see
// plan/ai/media/step-04-frontend-crop-upload-component.md. Renders the
// same hidden-input-plus-button shape as ToolInstallButton.tsx.
interface ImageUploadCropperProps {
  toolSlug: string
  // When set, a successful crop calls the REPLACE endpoint against this
  // existing image's own id, keeping it stable. When omitted, a
  // successful crop calls the CREATE endpoint instead.
  existingFileId?: string
  // Omitted means free-form cropping; pass aspect={1} for a square crop.
  aspect?: number
  // Human-readable label shown at the top of the crop modal (e.g.
  // "Cinqo", "Career") so the caller knows which tool_slug bucket
  // they're uploading into. Defaults to toolSlug itself when omitted.
  sourceLabel?: string
  onDone: (result: { fileId: string; width: number; height: number }) => void
  onError?: (message: string) => void
}

export function ImageUploadCropper({ toolSlug, existingFileId, aspect, sourceLabel, onDone, onError }: ImageUploadCropperProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [objectUrl, setObjectUrl] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const closeCropModal = () => {
    if (objectUrl) URL.revokeObjectURL(objectUrl)
    setObjectUrl(null)
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same file next time
    if (!file) return
    setObjectUrl(URL.createObjectURL(file))
  }

  const handleSave = async (croppedBlob: Blob) => {
    setBusy(true)
    try {
      const result = existingFileId ? await replaceImage(existingFileId, croppedBlob) : await uploadImage(croppedBlob, toolSlug)
      closeCropModal()
      onDone(result)
    } catch (err) {
      onError?.(err instanceof ApiError ? err.message : 'Failed to upload image.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <input ref={inputRef} type="file" accept="image/*" className="hidden" onChange={handleChange} disabled={busy} />
      <button
        type="button"
        onClick={() => inputRef.current?.click()}
        disabled={busy}
        className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
      >
        {busy ? 'Uploading…' : existingFileId ? 'Replace image' : 'Add Image'}
      </button>
      {objectUrl && (
        <CropModal
          imageSrc={objectUrl}
          aspect={aspect}
          sourceLabel={sourceLabel ?? toolSlug}
          onCancel={closeCropModal}
          onSave={(blob) => {
            void handleSave(blob)
          }}
        />
      )}
    </div>
  )
}
