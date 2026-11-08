import { useState } from 'react'
import { fetchProfilePhotos, mediaDownloadUrl, removeProfilePhoto, type ProfilePhoto } from '../../api'
import { PhotoActionButtons } from './PhotoActionButtons'
import { PhotoViewModal } from './PhotoViewModal'

// A profile's default photo as a round picture in a list item, with the view and remove icons on hover or focus
// (always on a touch screen), and the modal behind the view icon: the one piece the profiles list and the
// persona list both draw. View opens the details modal; remove takes the photo off the profile at once (it stays
// in the photo library, the newest remaining photo becomes the default), then calls onChanged so the page reads
// its data again. A failure is reported through onError. The page picks the size (a Tailwind size class, 48 px
// or more so the two icons fit).
export function PhotoAvatar({
  profileId,
  mediaFileId,
  sizeClass = 'h-12 w-12',
  onChanged,
  onError,
}: {
  profileId: string
  mediaFileId: string
  sizeClass?: string
  onChanged: () => void
  onError?: (message: string) => void
}) {
  const [viewing, setViewing] = useState<ProfilePhoto | null>(null)

  // The profile's photo entry (it knows when the photo was added), or just the picture when the list cannot be read.
  function view() {
    void (async () => {
      let found: ProfilePhoto | undefined
      try {
        found = (await fetchProfilePhotos(profileId)).find((p) => p.mediaFileId === mediaFileId)
      } catch {
        found = undefined
      }
      setViewing(found ?? { mediaFileId, isDefault: true, addedAt: '' })
    })()
  }

  function remove() {
    removeProfilePhoto(profileId, mediaFileId)
      .then(onChanged)
      .catch((err: unknown) => {
        onError?.(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <>
      <span className={`group relative block shrink-0 overflow-hidden rounded-full shadow-sm ${sizeClass}`}>
        <img src={mediaDownloadUrl(mediaFileId)} alt="" className="h-full w-full object-cover" />
        <span className="absolute inset-0 flex items-center justify-center gap-0.5 bg-black/40 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100">
          <PhotoActionButtons isDefault compact onView={view} onRemove={remove} />
        </span>
      </span>
      <PhotoViewModal
        photo={viewing}
        onClose={() => {
          setViewing(null)
        }}
      />
    </>
  )
}
