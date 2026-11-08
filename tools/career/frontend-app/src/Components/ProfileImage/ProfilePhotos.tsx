import { mediaDownloadUrl, type ProfilePhoto } from '../../api'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'

// The profile's own photos (step 111): the default is the one the CV templates use. Make another
// the default, or take one off the profile (it stays in the library). A uniquely-named grid class,
// as the other Career pages do: the globally-injected Tailwind stylesheet can collapse `.grid-cols-*`.
export function ProfilePhotos({
  photos,
  busy,
  onMakeDefault,
  onRemove,
}: {
  photos: ProfilePhoto[]
  busy: boolean
  onMakeDefault: (photo: ProfilePhoto) => void
  onRemove: (photo: ProfilePhoto) => void
}) {
  if (photos.length === 0) return null
  return (
    <section
      aria-label="This profile's photos"
      className="career-enter career-delay-2 rounded-3xl border bg-white p-5 shadow-sm"
      style={{ borderColor: PAGE_ACCENT.soft }}
    >
      <style>{`
        .profile-photos-grid { display: grid; gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(7.5rem, 1fr)); }
      `}</style>
      <h2 className="text-sm font-semibold text-gray-700">This profile&apos;s photos</h2>
      <p className="mb-3 mt-0.5 text-xs text-gray-500">The default is used on a CV unless you pick another when you create it.</p>
      <ul className="profile-photos-grid">
        {photos.map((photo) => (
          <li key={photo.mediaFileId} className="flex flex-col gap-1.5">
            <span
              className={`relative block aspect-square overflow-hidden rounded-2xl border-2 bg-gray-100 ${photo.isDefault ? 'shadow-md' : 'border-transparent'}`}
              style={photo.isDefault ? { borderColor: PAGE_ACCENT.to } : undefined}
            >
              <img src={mediaDownloadUrl(photo.mediaFileId)} alt="" className="h-full w-full object-cover" />
              {photo.isDefault && (
                <span
                  className="absolute bottom-1 left-1 rounded-full px-2 py-0.5 text-[10px] font-semibold text-white"
                  style={{ background: PAGE_ACCENT.to }}
                >
                  Default
                </span>
              )}
            </span>
            <div className="flex flex-wrap gap-1">
              {!photo.isDefault && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    onMakeDefault(photo)
                  }}
                  className="rounded-full border border-blue-200 px-2.5 py-0.5 text-xs text-blue-700 hover:bg-blue-50 disabled:opacity-50"
                >
                  Make default
                </button>
              )}
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  onRemove(photo)
                }}
                title="Take this photo off the profile. It stays in your library."
                className="rounded-full border border-gray-200 px-2.5 py-0.5 text-xs text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Remove
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
