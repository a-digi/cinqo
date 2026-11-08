import { useEffect, useState } from 'react'
import { fetchPhotos, mediaDownloadUrl, type ProfilePhoto } from '../../api'
import { httpClient } from '../../Cinqo/Http/client'
import { Modal } from '../../Shared/Modal/Modal'
import { photoMetaRows, type MediaMeta } from './photoMeta'

function formatDateTime(value: string): string {
  const date = new Date(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) ? `${value.replace(' ', 'T')}Z` : value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}

// The photo's metadata from core's media list (the calling user's own Career files).
async function fetchMediaMeta(mediaFileId: string): Promise<MediaMeta | null> {
  const res = await httpClient.get('/api/v1/media/mine?toolSlug=career')
  const list = await httpClient.responseBody<MediaMeta[]>(res, 'load media')
  return list.find((m) => m.id === mediaFileId) ?? null
}

// One photo of a profile in a modal: the image at a readable size and its details (default or not, when it
// was added, file name, type, size, dimensions, which profiles use it). What cannot be read is left out;
// the image is shown regardless.
export function PhotoViewModal({ photo, onClose }: { photo: ProfilePhoto | null; onClose: () => void }) {
  const [meta, setMeta] = useState<MediaMeta | null>(null)
  const [usedBy, setUsedBy] = useState<string[]>([])
  const mediaFileId = photo?.mediaFileId

  useEffect(() => {
    setMeta(null)
    setUsedBy([])
    if (mediaFileId === undefined) return
    let cancelled = false
    fetchMediaMeta(mediaFileId)
      .then((m) => {
        if (!cancelled) setMeta(m)
      })
      .catch(() => undefined)
    fetchPhotos()
      .then((all) => {
        if (!cancelled) setUsedBy(all.find((p) => p.mediaFileId === mediaFileId)?.usedBy.map((u) => u.name) ?? [])
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [mediaFileId])

  const rows = photo ? photoMetaRows({ isDefault: photo.isDefault, addedAt: photo.addedAt, meta, usedBy, format: formatDateTime }) : []
  return (
    <Modal open={photo !== null} title={photo?.isDefault ? 'Default photo' : 'Photo'} onClose={onClose} size="wide">
      {photo && (
        <div className="grid gap-5 sm:grid-cols-[minmax(0,1fr)_16rem]">
          <img src={mediaDownloadUrl(photo.mediaFileId)} alt="" className="max-h-[60vh] w-full rounded-xl bg-gray-100 object-contain" />
          <dl className="space-y-2.5 text-sm">
            {rows.map((r) => (
              <div key={r.label} className="min-w-0">
                <dt className="text-[11px] font-medium uppercase tracking-wide text-gray-400">{r.label}</dt>
                <dd className="mt-0.5 break-words text-gray-800">{r.value}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </Modal>
  )
}
