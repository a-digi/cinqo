import { useState } from 'react'
import type { ProfilePhoto } from '../../api'
import { PhotoThumbGrid } from './PhotoThumbGrid'
import { PhotoViewModal } from './PhotoViewModal'

// A profile's photos with their view and remove icons and the modal behind the view icon: the one piece
// the profile view and the persona view both draw (the heading and the frame are the page's own). The list
// and the removing come from useProfilePhotos.
export function ProfilePhotosPanel({
  photos,
  error,
  onRemove,
}: {
  photos: ProfilePhoto[]
  error: string
  onRemove: (photo: ProfilePhoto) => void
}) {
  const [viewing, setViewing] = useState<ProfilePhoto | null>(null)
  return (
    <>
      {error !== '' && <p className="mb-2 text-xs text-red-700">{error}</p>}
      {photos.length === 0 ? (
        <p className="text-sm text-gray-400">No photos yet.</p>
      ) : (
        <PhotoThumbGrid photos={photos} onView={setViewing} onRemove={onRemove} />
      )}
      <PhotoViewModal
        photo={viewing}
        onClose={() => {
          setViewing(null)
        }}
      />
    </>
  )
}
