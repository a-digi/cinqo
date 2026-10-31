import { mediaDownloadUrl, type ProfilePhoto } from '../../api'
import { EyeIcon, TrashIcon } from '../../Shared/Icons/icons'

const floatingButton =
  'flex h-7 w-7 items-center justify-center rounded-lg bg-white text-gray-700 shadow transition-colors hover:bg-blue-50 hover:text-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500'

// A grid of a profile's photos as square thumbnails, the default marked. On hover or focus (always on a
// touch screen) each thumbnail shows a view icon and, when onRemove is given, a remove icon. Meant for any
// page that lists a profile's photos read-only; the actions are the page's own. A uniquely-named grid
// class, as the other Career pages do: the globally-injected Tailwind stylesheet can collapse `.grid-cols-*`.
export function PhotoThumbGrid({
  photos,
  onView,
  onRemove,
}: {
  photos: ProfilePhoto[]
  onView: (photo: ProfilePhoto) => void
  onRemove?: (photo: ProfilePhoto) => void
}) {
  const controls = 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100'
  return (
    <>
      <style>{`
        .photo-thumb-grid { display: grid; gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(5.5rem, 1fr)); }
      `}</style>
      <ul className="photo-thumb-grid">
        {photos.map((photo) => (
          <li key={photo.mediaFileId} className="group relative aspect-square overflow-hidden rounded-xl bg-gray-100">
            <img
              src={mediaDownloadUrl(photo.mediaFileId)}
              alt={photo.isDefault ? 'Default' : ''}
              className={`h-full w-full object-cover ${photo.isDefault ? 'ring-2 ring-inset ring-purple-400' : ''}`}
            />
            {photo.isDefault && (
              <span className="absolute bottom-1 left-1 rounded-full bg-purple-600 px-1.5 py-0.5 text-[9px] font-semibold text-white">
                Default
              </span>
            )}
            <div className={`absolute right-1 top-1 flex items-center gap-1 transition-opacity ${controls}`}>
              <button
                type="button"
                aria-label={photo.isDefault ? 'View the default photo' : 'View this photo'}
                title="View"
                onClick={() => {
                  onView(photo)
                }}
                className={floatingButton}
              >
                <EyeIcon />
              </button>
              {onRemove && (
                <button
                  type="button"
                  aria-label={photo.isDefault ? 'Remove the default photo from this profile' : 'Remove this photo from this profile'}
                  title="Remove from this profile (it stays in the photo library)"
                  onClick={() => {
                    onRemove(photo)
                  }}
                  className={`${floatingButton} hover:bg-red-50 hover:text-red-700`}
                >
                  <TrashIcon />
                </button>
              )}
            </div>
          </li>
        ))}
      </ul>
    </>
  )
}
