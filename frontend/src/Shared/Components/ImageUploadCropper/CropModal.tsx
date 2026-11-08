import { useState } from 'react'
import Cropper from 'react-easy-crop'
import type { Area, Point } from 'react-easy-crop'
import { Pill } from '../Pill/Pill'
import { XIcon } from '../IconButton/icons'

// See plan/ai/media/step-04-frontend-crop-upload-component.md — the
// draggable/zoomable overlay. onCropComplete hands back pixel
// coordinates in the ORIGINAL image's own pixel space, which Save
// below draws onto an offscreen canvas to produce the final cropped
// Blob. No resizing happens here beyond what cropping itself does —
// the server's own ResizeToMax is the authoritative size cap.
interface CropModalProps {
  imageSrc: string
  aspect?: number
  // Shown in the modal's own header — which tool_slug bucket this
  // upload/replace is going into (see ImageUploadCropper's own doc
  // comment for sourceLabel).
  sourceLabel: string
  // True for a new upload (title is required — see
  // plan/ai/media/step-10-obligatory-title.md); false when replacing an
  // existing image's own bytes, which keeps its own existing title
  // untouched, so no title input renders at all in that case.
  requireTitle: boolean
  onCancel: () => void
  onSave: (croppedBlob: Blob, title?: string) => void
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.addEventListener('load', () => {
      resolve(img)
    })
    img.addEventListener('error', () => {
      reject(new Error('Failed to load image.'))
    })
    img.src = src
  })
}

async function cropToBlob(imageSrc: string, area: Area): Promise<Blob> {
  const img = await loadImage(imageSrc)
  const canvas = document.createElement('canvas')
  canvas.width = area.width
  canvas.height = area.height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Canvas is not supported.')
  ctx.drawImage(img, area.x, area.y, area.width, area.height, 0, 0, area.width, area.height)
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('Failed to encode cropped image.'))
    }, 'image/jpeg')
  })
}

export function CropModal({ imageSrc, aspect, sourceLabel, requireTitle, onCancel, onSave }: CropModalProps) {
  const [crop, setCrop] = useState<Point>({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [croppedAreaPixels, setCroppedAreaPixels] = useState<Area | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  // react-easy-crop always needs a numeric aspect — there is no true
  // arbitrary-shape crop area. "aspect omitted" (free-form) is
  // approximated by matching the SOURCE image's own natural ratio
  // (known only once it loads), so a caller that doesn't force a
  // specific ratio (e.g. a square avatar) still gets a crop box shaped
  // like their own picture, not an unrelated forced square.
  const [naturalAspect, setNaturalAspect] = useState<number | null>(null)
  const effectiveAspect = aspect ?? naturalAspect ?? 1

  const trimmedTitle = title.trim()
  const canSave = croppedAreaPixels !== null && (!requireTitle || trimmedTitle !== '')

  const handleSave = async () => {
    if (!croppedAreaPixels || (requireTitle && trimmedTitle === '')) return
    setSaving(true)
    setError(null)
    try {
      const blob = await cropToBlob(imageSrc, croppedAreaPixels)
      onSave(blob, requireTitle ? trimmedTitle : undefined)
    } catch {
      setError('Failed to crop image.')
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex w-full max-w-lg flex-col overflow-hidden rounded-2xl bg-white shadow-2xl">
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold text-gray-900">Crop image</h2>
            <Pill outline>{sourceLabel}</Pill>
          </div>
          <button
            type="button"
            onClick={onCancel}
            disabled={saving}
            aria-label="Close"
            className="rounded-full p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50"
          >
            <XIcon />
          </button>
        </div>

        <div className="p-5">
          <div className="relative h-80 w-full overflow-hidden rounded-xl bg-gray-900">
            <Cropper
              image={imageSrc}
              crop={crop}
              zoom={zoom}
              aspect={effectiveAspect}
              onCropChange={setCrop}
              onZoomChange={setZoom}
              onCropComplete={(_area, areaPixels) => {
                setCroppedAreaPixels(areaPixels)
              }}
              onMediaLoaded={(size) => {
                if (aspect === undefined) setNaturalAspect(size.naturalWidth / size.naturalHeight)
              }}
            />
          </div>

          <div className="mt-4 flex items-center gap-3">
            <span className="text-xs font-medium text-gray-500">Zoom</span>
            <input
              type="range"
              min={1}
              max={3}
              step={0.01}
              value={zoom}
              onChange={(e) => {
                setZoom(Number(e.target.value))
              }}
              className="w-full accent-gray-900"
            />
          </div>

          {requireTitle && (
            <div className="mt-4">
              <label htmlFor="crop-modal-title" className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Title
              </label>
              <input
                id="crop-modal-title"
                type="text"
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value)
                }}
                placeholder="Required"
                className="mt-1 w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-gray-900 focus:outline-none"
              />
            </div>
          )}

          {error && <p className="mt-3 text-sm text-red-600">{error}</p>}
        </div>

        <div className="flex justify-end gap-2 border-t border-gray-100 bg-gray-50 px-5 py-4">
          <button
            type="button"
            onClick={onCancel}
            disabled={saving}
            className="rounded-md border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              void handleSave()
            }}
            disabled={saving || !canSave}
            className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
