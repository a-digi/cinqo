import { useCallback, useEffect, useState } from 'react'
import { fetchProfilePhotos, removeProfilePhoto, type ProfilePhoto } from '../../api'

// A profile's photos for a page that shows them (the profile view, the persona view): the list, and removing
// one from the profile. Removing takes the photo off the profile right away (it stays in the photo library,
// where it can be added again; if it was the default, the newest remaining photo becomes the default), then
// reads the list again and calls onChanged, so a page showing the default photo elsewhere can refresh it.
// A list that cannot be read is empty (best effort).
export function useProfilePhotos(profileId: string | null, onChanged?: () => void) {
  const [photos, setPhotos] = useState<ProfilePhoto[]>([])
  const [error, setError] = useState('')

  // Reads the list again (a best effort: a list that cannot be read keeps what is shown).
  const reload = useCallback(() => {
    if (!profileId) return
    fetchProfilePhotos(profileId)
      .then(setPhotos)
      .catch(() => undefined)
  }, [profileId])

  useEffect(() => {
    setPhotos([])
    setError('')
    reload()
  }, [reload])

  const remove = useCallback(
    (photo: ProfilePhoto) => {
      if (!profileId) return
      setError('')
      removeProfilePhoto(profileId, photo.mediaFileId)
        .then(() => fetchProfilePhotos(profileId))
        .then((list) => {
          setPhotos(list)
          onChanged?.()
        })
        .catch((err: unknown) => {
          setError(err instanceof Error ? err.message : String(err))
        })
    },
    [profileId, onChanged],
  )

  return { photos, error, remove, reload }
}
