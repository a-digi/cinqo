import { useState } from 'react'
import Cropper from 'react-easy-crop'
import type { Area, Point } from 'react-easy-crop'

// See plan/ai/media/step-04-frontend-crop-upload-component.md — the
// draggable/zoomable overlay. onCropComplete hands back pixel
// coordinates in the ORIGINAL image's own pixel space, which Save
// below draws onto an offscreen canvas to produce the final cropped
// Blob. No resizing happens here beyond what cropping itself does —
// the server's own ResizeToMax is the authoritative size cap.
interface CropModalProps {
  imageSrc: string
  aspect?: number
  onCancel: () => void
  onSave: (croppedBlob: Blob) => void
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

export function CropModal({ imageSrc, aspect, onCancel, onSave }: CropModalProps) {
  const [crop, setCrop] = useState<Point>({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [croppedAreaPixels, setCroppedAreaPixels] = useState<Area | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // react-easy-crop always needs a numeric aspect — there is no true
  // arbitrary-shape crop area. "aspect omitted" (free-form) is
  // approximated by matching the SOURCE image's own natural ratio
  // (known only once it loads), so a caller that doesn't force a
  // specific ratio (e.g. a square avatar) still gets a crop box shaped
  // like their own picture, not an unrelated forced square.
  const [naturalAspect, setNaturalAspect] = useState<number | null>(null)
  const effectiveAspect = aspect ?? naturalAspect ?? 1

  const handleSave = async () => {
    if (!croppedAreaPixels) return
    setSaving(true)
    setError(null)
    try {
      const blob = await cropToBlob(imageSrc, croppedAreaPixels)
      onSave(blob)
    } catch {
      setError('Failed to crop image.')
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div className="flex w-full max-w-lg flex-col rounded-xl bg-white p-4 shadow-lg">
        <div className="relative h-80 w-full overflow-hidden rounded-lg bg-gray-900">
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
        <input
          type="range"
          min={1}
          max={3}
          step={0.01}
          value={zoom}
          onChange={(e) => {
            setZoom(Number(e.target.value))
          }}
          className="mt-4 accent-gray-900"
        />
        {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={saving}
            className="rounded-md border border-gray-200 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              void handleSave()
            }}
            disabled={saving || !croppedAreaPixels}
            className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
